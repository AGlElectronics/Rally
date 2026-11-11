// Package http wires the chi router and ties handlers to their
// dependencies. The structure deliberately mirrors Stash: one Deps struct
// consumed by one NewRouter, with per-resource handler types owning their
// own routing closure.
//
// API shape:
//
//	GET /livez             — process liveness (unauthenticated)
//	GET /readyz            — service readiness (unauthenticated)
//	GET /api/health        — richer status for humans
//	GET /api/role-catalog  — roles for HR (HR_SERVICE_TOKEN or a staff sign-in)
//	GET /api/den/state     — Den mode and pairing (unauthenticated; also /api/v1/den/state)
//	GET /api/v1/brand      — active brand package (unauthenticated)
//	GET /api/v1/me         — authenticated principal + permissions
//	GET /api/v1/organizations  — list visible organizations
//	GET /api/v1/organizations/{id} — read a single organization
//	GET /api/v1/editions — editions visible to the caller
//	GET|POST /api/v1/editions/{edition}/cfps
//	GET|PUT|DELETE /api/v1/cfps/{cfp}
//	GET /api/v1/editions/{edition}/submissions
//	POST /api/v1/cfps/{cfp}/submissions
//	GET|PATCH /api/v1/submissions/{submission}
//	POST /api/v1/submissions/{submission}/{submit|withdraw|accept|reject}
//	GET|POST /api/v1/submissions/{submission}/notes
//	GET|PUT /api/v1/submissions/{submission}/scores
//	GET|POST /api/v1/editions/{edition}/event-types, PUT|DELETE /api/v1/event-types/{id}
//	POST /api/v1/uploads, POST /api/v1/uploads/{file}/complete, GET /api/v1/files/{file}/download
//	PUT /api/v1/editions/{edition}/dates, GET|POST .../rooms, PUT|DELETE /api/v1/rooms/{room}
//	GET /api/v1/editions/{edition}/schedule, PUT .../schedule/settings
//	POST .../schedule/items, PATCH|DELETE /api/v1/schedule-items/{item}
//	POST .../schedule/publish, GET .../schedule/versions[/{version}], POST .../versions/{version}/restore
//	POST .../schedule/live/{item}  — delay / cancel / restore / move on the published schedule
//	GET|POST .../announcements, PUT /api/v1/announcements/{id}, POST .../{id}/{send|discard}
//	GET|POST .../telegram-chats, PUT|DELETE /api/v1/telegram-chats/{chat}
//
// The public submission site talks to /api/public/v1 with tokens from the
// attendee issuer, never the staff one:
//
//	GET /api/public/v1/calls[/{cfp}]  — open calls (unauthenticated)
//	GET /api/public/v1/schedules[/{edition}[/calendar.ics]] — published programme
//	    (unauthenticated; 404 unless switched on in Den or PUBLIC_SCHEDULE_ENABLED)
//	GET /api/public/v1/me             — attendee profile + presenter code
//	GET /api/public/v1/submissions    — mine (submitter or co-presenter)
//	POST /api/public/v1/calls/{cfp}/submissions, GET|PATCH .../submissions/{id}
//	POST .../submissions/{id}/{submit|withdraw|leave}
//	POST|DELETE .../submissions/{id}/people[/{person}], POST .../people/{person}/reissue
//	GET /api/public/v1/invites/{code}, POST .../invites/{code}/{accept|decline}
//	POST .../uploads, POST .../uploads/{file}/complete, GET .../files/{file}/download
//
// Everything under /api/v1 (except /brand) requires a staff token; the
// auth middleware is applied inside a chi.Group so the shape is explicit.
// With DEN_MODE=required, the /api routes other than health, role catalog,
// brand, me and den answer 503 den_pairing_required until Den pairs.
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/authbase"
	"github.com/AGlElectronics/rally/backend/internal/brand"
	"github.com/AGlElectronics/rally/backend/internal/clientip"
	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/denlink"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/files"
	"github.com/AGlElectronics/rally/backend/internal/telegram"
)

// Deps groups the runtime dependencies the HTTP layer needs. Files is
// optional (scaffold boots without object storage when S3_ENABLED=false).
type Deps struct {
	Config       *config.Config
	Pool         *pgxpool.Pool
	Logger       zerolog.Logger
	Files        *files.Client // may be nil when S3_ENABLED=false
	Brand        brand.Provider
	OIDCVerifier *auth.OIDCVerifier
	// AttendeeVerifier checks tokens of the attendee realm (public site);
	// nil = attendee sign-in not configured.
	AttendeeVerifier *auth.OIDCVerifier
	Access           *auth.AccessConfig
	// Den is the pairing with Den; nil means a fresh standalone client.
	Den *denlink.Client
	// FileService is shared with the upload sweeper; nil = built here.
	FileService *events.FileService
	// Keys guards the public schedule API; nil = built from Pool.
	Keys *events.KeyService
}

// NewRouter builds the chi router with middleware, CORS, and routes.
// Version is passed in from cmd/server at build time.
func NewRouter(deps Deps, version string) http.Handler {
	r := chi.NewRouter()
	authCfg := deps.Config.Auth()

	r.Use(authbase.SecurityHeaders)
	r.Use(authCfg.StripDevHeaders)
	r.Use(chimiddleware.RequestID)
	r.Use(clientip.Middleware(deps.Config.TrustedProxyCIDRs))
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))

	r.Use(hlog.NewHandler(deps.Logger))
	r.Use(hlog.AccessHandler(func(req *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(req).Info().
			Str("method", req.Method).
			Str("path", req.URL.Path).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Msg("http")
	}))
	r.Use(hlog.RequestIDHandler("request_id", "X-Request-ID"))

	// AllowedHeaders has X-Dev-User and X-Dev-Groups only in dev mode.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   deps.Config.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   authCfg.AllowedHeaders(),
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	health := newHealthHandler(deps.Pool, version)
	resolver := auth.NewResolver(deps.Pool, deps.Access, deps.Config.Groups)
	authMW := auth.Middleware(deps.Config, resolver, deps.OIDCVerifier)

	den := deps.Den
	if den == nil {
		denCfg := deps.Config.Den
		denCfg.Slug, denCfg.Version = config.DenSlug, version
		den = denlink.New(deps.Pool, denCfg, deps.Logger)
	}
	brandH := &brandHandler{provider: deps.Brand, den: den}
	orgs := &organizationHandler{svc: events.NewOrganizationService(deps.Pool)}
	editions := &editionHandler{svc: events.NewEditionService(deps.Pool)}
	cfps := &cfpHandler{svc: events.NewCfPService(deps.Pool)}
	subSvc := events.NewSubmissionService(deps.Pool)
	subs := &submissionHandler{svc: subSvc}
	eventTypes := &eventTypeHandler{svc: events.NewEventTypeService(deps.Pool)}
	fileSvc := deps.FileService
	if fileSvc == nil {
		fileSvc = events.NewFileService(deps.Pool, deps.Files)
	}
	filesH := &fileHandler{svc: fileSvc}
	public := &publicHandler{public: events.NewPublicService(deps.Pool), subs: subSvc}
	sched := &scheduleHandler{
		rooms: events.NewRoomService(deps.Pool),
		svc: events.NewScheduleService(deps.Pool,
			telegram.New(deps.Config.TelegramBotToken, deps.Config.TelegramAPIURL),
			publicScheduleSwitch(den, deps.Config.PublicScheduleEnabled)),
		productName: func() string {
			if n := den.OrganizationName(); n != "" {
				return n
			}
			return "Rally"
		},
	}
	attendeeMW := auth.AttendeeMiddleware(deps.Config, resolver, deps.AttendeeVerifier)
	keySvc := deps.Keys
	if keySvc == nil {
		keySvc = events.NewKeyService(deps.Pool)
	}
	apiKeys := &apiKeyHandler{keys: keySvc}

	// Per replica, in memory: the public site per client IP, the endpoints
	// that create things per person (else per IP).
	publicLimit := rateLimiter(deps.Config.PublicRateLimit, authbase.ClientIP)
	submitLimit := rateLimiter(deps.Config.SubmissionRateLimit, authbase.BySubject(userKey))

	// Orchestrator probes — unauthenticated, served from the root so they
	// don't collide with any /api/* middleware.
	r.Get("/livez", health.livez)
	r.Get("/readyz", health.readyz)

	r.Route("/api", func(api chi.Router) {
		api.Use(den.RequirePaired)
		api.Get("/health", health.get)
		api.With(requireCatalogReader(deps.Config.HRServiceToken, deps.Config.DevAuth(), authMW)).
			Get("/role-catalog", roleCatalogHandler(deps.Access))
		api.Get("/den/state", den.StateHandler)

		// Public submission site. Only attendee-realm tokens work here, and
		// attendees never carry staff permissions.
		api.Route("/public/v1", func(pub chi.Router) {
			pub.Use(publicLimit)
			pub.Get("/brand", brandH.get)
			pub.Get("/brand/{role}", den.AssetHandler)
			pub.Get("/calls", public.calls)
			pub.Get("/calls/{cfp}", public.call)
			// Switched on by Den's events_public_schedule (else
			// PUBLIC_SCHEDULE_ENABLED), and authenticated by an API key
			// managed under /api/v1/public-api-keys.
			pub.Group(func(ps chi.Router) {
				ps.Use(sched.requirePublicSchedule)
				ps.Use(requirePublicKey(keySvc))
				ps.Get("/schedules", sched.publicList)
				ps.Get("/schedules/{edition}", sched.publicSchedule)
				ps.Get("/schedules/{edition}/calendar.ics", sched.publicICS)
			})
			pub.Group(func(a chi.Router) {
				a.Use(attendeeMW)
				a.Get("/me", public.me)
				a.Get("/submissions", public.mine)
				a.With(submitLimit).Post("/calls/{cfp}/submissions", subs.create)
				a.Get("/submissions/{submission}", subs.get)
				a.Patch("/submissions/{submission}", subs.update)
				a.With(submitLimit).Post("/submissions/{submission}/submit", subs.submit)
				a.Post("/submissions/{submission}/withdraw", subs.withdraw)
				a.Post("/submissions/{submission}/leave", public.leave)
				a.With(submitLimit).Post("/submissions/{submission}/people", public.addPerson)
				a.Delete("/submissions/{submission}/people/{person}", public.removePerson)
				a.With(submitLimit).Post("/submissions/{submission}/people/{person}/reissue", public.reissueInvite)
				// Invite codes are short: guessing them is what this limit stops.
				a.With(submitLimit).Get("/invites/{code}", public.invite)
				a.With(submitLimit).Post("/invites/{code}/accept", public.acceptInvite)
				a.With(submitLimit).Post("/invites/{code}/decline", public.declineInvite)
				a.With(submitLimit).Post("/uploads", filesH.start)
				a.Post("/uploads/{file}/complete", filesH.complete)
				a.Get("/files/{file}/download", filesH.download)
			})
		})

		api.Route("/v1", func(v1 chi.Router) {
			// Brand is public so the login screen can render before the
			// user authenticates.
			v1.Get("/brand", brandH.get)
			v1.Get("/brand/{role}", den.AssetHandler)
			v1.Get("/den/state", den.StateHandler)

			v1.Group(func(secure chi.Router) {
				secure.Use(authMW)
				secure.Get("/me", meHandler)
				prefs := den.PreferencesHandler(func(r *http.Request) (string, string, bool) {
					u, ok := auth.FromContext(r.Context())
					if !ok || u.Issuer == "" || u.Subject == "" {
						return "", "", false
					}
					return u.Issuer, u.Subject, true
				})
				secure.Get("/me/preferences", prefs)
				secure.Put("/me/preferences", prefs)
				secure.Route("/den", func(dr chi.Router) {
					dr.Use(requirePermission("org.manage"))
					dr.Get("/", den.StatusHandler)
					dr.Post("/pair", den.PairHandler)
					dr.Post("/sync", den.SyncHandler)
					dr.Delete("/pair", den.UnpairHandler)
				})
				secure.Get("/organizations", orgs.list)
				secure.Get("/organizations/{id}", orgs.get)

				secure.Get("/editions", editions.list)
				secure.Route("/editions/{edition}", func(ed chi.Router) {
					ed.Get("/cfps", cfps.list)
					ed.Post("/cfps", cfps.create)
					ed.Get("/submissions", subs.listForEdition)
					ed.Get("/event-types", eventTypes.list)
					ed.Post("/event-types", eventTypes.create)
					ed.Put("/dates", sched.updateDates)
					ed.Get("/rooms", sched.listRooms)
					ed.Post("/rooms", sched.createRoom)
					ed.Get("/schedule", sched.get)
					ed.Put("/schedule/settings", sched.updateSettings)
					ed.Post("/schedule/items", sched.createItem)
					ed.Post("/schedule/publish", sched.publish)
					ed.Get("/schedule/versions", sched.versions)
					ed.Get("/schedule/versions/{version}", sched.version)
					ed.Post("/schedule/versions/{version}/restore", sched.restore)
					ed.Post("/schedule/live/{item}", sched.live)
					ed.Get("/announcements", sched.announcements)
					ed.Post("/announcements", sched.createAnnouncement)
					ed.Get("/telegram-chats", sched.chats)
					ed.Post("/telegram-chats", sched.createChat)
				})
				secure.Put("/rooms/{room}", sched.updateRoom)
				secure.Delete("/rooms/{room}", sched.deleteRoom)
				secure.Patch("/schedule-items/{item}", sched.updateItem)
				secure.Delete("/schedule-items/{item}", sched.deleteItem)
				secure.Put("/announcements/{announcement}", sched.updateAnnouncement)
				secure.Post("/announcements/{announcement}/send", sched.sendAnnouncement)
				secure.Post("/announcements/{announcement}/discard", sched.discardAnnouncement)
				secure.Put("/telegram-chats/{chat}", sched.updateChat)
				secure.Delete("/telegram-chats/{chat}", sched.deleteChat)
				secure.Put("/event-types/{eventType}", eventTypes.update)
				secure.Delete("/event-types/{eventType}", eventTypes.delete)

				secure.Get("/submissions/{submission}/notes", subs.notes)
				secure.Post("/submissions/{submission}/notes", subs.addNote)
				secure.Get("/submissions/{submission}/scores", subs.scores)
				secure.Put("/submissions/{submission}/scores", subs.setScore)
				secure.Post("/uploads", filesH.start)
				secure.Post("/uploads/{file}/complete", filesH.complete)
				secure.Get("/files/{file}/download", filesH.download)

				secure.Get("/cfps/{cfp}", cfps.get)
				secure.Put("/cfps/{cfp}", cfps.update)
				secure.Delete("/cfps/{cfp}", cfps.delete)
				secure.With(submitLimit).Post("/cfps/{cfp}/submissions", subs.create)

				secure.Get("/submissions/{submission}", subs.get)
				secure.Patch("/submissions/{submission}", subs.update)
				secure.Post("/submissions/{submission}/submit", subs.submit)
				secure.Post("/submissions/{submission}/withdraw", subs.withdraw)
				secure.Post("/submissions/{submission}/accept", subs.accept)
				secure.Post("/submissions/{submission}/reject", subs.reject)

				// Keys for the public schedule API (shown once on create).
				secure.Route("/public-api-keys", func(kr chi.Router) {
					kr.Use(requirePermission(domain.PermSchedulePublish))
					kr.Get("/", apiKeys.list)
					kr.Post("/", apiKeys.create)
					kr.Delete("/{key}", apiKeys.revoke)
				})
			})
		})
	})

	return r
}

// rateLimiter is an authbase limiter for l, or a pass-through when l is off.
func rateLimiter(l config.RateLimit, key authbase.KeyFunc) func(http.Handler) http.Handler {
	if l.Rate <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return authbase.NewRateLimiter(authbase.RateLimitOptions{Rate: l.Rate, Burst: l.Burst, Key: key}).Middleware
}

// userKey is the signed-in person (staff or attendee) of a request.
func userKey(r *http.Request) (authbase.UserKey, bool) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		return authbase.UserKey{}, false
	}
	return authbase.UserKey{Issuer: u.Issuer, Subject: u.Subject}, true
}
