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
//
// Everything under /api/v1 (except /brand) requires authentication; the
// auth middleware is applied inside a chi.Group so the shape is explicit.
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
	"github.com/AGlElectronics/rally/backend/internal/brand"
	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/files"
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
	Access       *auth.AccessConfig
}

// NewRouter builds the chi router with middleware, CORS, and routes.
// Version is passed in from cmd/server at build time.
func NewRouter(deps Deps, version string) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
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

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   deps.Config.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Dev-User", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	health := newHealthHandler(deps.Pool, version)
	resolver := auth.NewResolver(deps.Pool, deps.Access)
	authMW := auth.Middleware(deps.Config, resolver, deps.OIDCVerifier)

	brandH := &brandHandler{provider: deps.Brand}
	orgs := &organizationHandler{svc: events.NewOrganizationService(deps.Pool)}
	editions := &editionHandler{svc: events.NewEditionService(deps.Pool)}
	cfps := &cfpHandler{svc: events.NewCfPService(deps.Pool)}
	subs := &submissionHandler{svc: events.NewSubmissionService(deps.Pool)}

	// Orchestrator probes — unauthenticated, served from the root so they
	// don't collide with any /api/* middleware.
	r.Get("/livez", health.livez)
	r.Get("/readyz", health.readyz)

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", health.get)
		api.Get("/role-catalog", roleCatalogHandler(deps.Access))

		api.Route("/v1", func(v1 chi.Router) {
			// Brand is public so the login screen can render before the
			// user authenticates.
			v1.Get("/brand", brandH.get)

			v1.Group(func(secure chi.Router) {
				secure.Use(authMW)
				secure.Get("/me", meHandler)
				secure.Get("/organizations", orgs.list)
				secure.Get("/organizations/{id}", orgs.get)

				secure.Get("/editions", editions.list)
				secure.Route("/editions/{edition}", func(ed chi.Router) {
					ed.Get("/cfps", cfps.list)
					ed.Post("/cfps", cfps.create)
					ed.Get("/submissions", subs.listForEdition)
				})

				secure.Get("/cfps/{cfp}", cfps.get)
				secure.Put("/cfps/{cfp}", cfps.update)
				secure.Delete("/cfps/{cfp}", cfps.delete)
				secure.Post("/cfps/{cfp}/submissions", subs.create)

				secure.Get("/submissions/{submission}", subs.get)
				secure.Patch("/submissions/{submission}", subs.update)
				secure.Post("/submissions/{submission}/submit", subs.submit)
				secure.Post("/submissions/{submission}/withdraw", subs.withdraw)
				secure.Post("/submissions/{submission}/accept", subs.accept)
				secure.Post("/submissions/{submission}/reject", subs.reject)
			})
		})
	})

	return r
}
