package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	"github.com/AGlElectronics/rally/backend/internal/codes"
	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// DevAttendeeIssuer is the reserved issuer for attendees in AUTH_MODE=dev.
const DevAttendeeIssuer = "dev-attendee"

// AttendeeMiddleware authenticates the public submission site. Attendees come
// from their own realm and never carry staff permissions: group claims are
// ignored and no memberships are loaded, so a staff permission check on an
// attendee always fails.
//
// verifier is nil when ATTENDEE_OIDC_ISSUER_URL is unset; in oidc mode every
// request then gets a 503 so the site can say sign-in isn't set up.
func AttendeeMiddleware(cfg *config.Config, resolver *Resolver, verifier *OIDCVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			var (
				user *User
				err  error
			)
			switch {
			case cfg.DevAuth():
				header := strings.TrimSpace(r.Header.Get("X-Dev-User"))
				if header == "" {
					header = "dev-attendee"
				}
				user, err = resolver.loadOrProvisionAttendee(ctx, DevAttendeeIssuer, header,
					displayFromDevHeader(header), header+"@attendee.local", true)
			case cfg.AuthMode == config.AuthModeOIDC:
				if verifier == nil {
					httperror.Render(w, r, httperror.New(httperror.CodeServiceUnavailable, "attendee sign-in is not configured"))
					return
				}
				bearer := bearerFromHeader(r.Header.Get("Authorization"))
				if bearer == "" {
					httperror.Render(w, r, httperror.New(httperror.CodeUnauthorized, "missing bearer token"))
					return
				}
				claims, verr := verifier.Verify(ctx, bearer)
				if verr != nil {
					log.Ctx(ctx).Debug().Err(verr).Msg("attendee token verify failed")
					httperror.Render(w, r, httperror.New(httperror.CodeUnauthorized, "invalid token"))
					return
				}
				user, err = resolver.loadOrProvisionAttendee(ctx, claims.Issuer, claims.Subject,
					displayNameFromClaims(claims), strings.TrimSpace(claims.Email), claims.EmailVerified)
			default:
				httperror.Render(w, r, httperror.Internal(nil))
				return
			}
			if err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("attendee resolver failed")
				httperror.Render(w, r, httperror.Internal(err))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(ctx, user)))
		})
	}
}

func (r *Resolver) loadOrProvisionAttendee(ctx context.Context, issuer, subject, display, email string, verified bool) (*User, error) {
	if issuer == "" || subject == "" {
		return nil, errors.New("attendee identity missing issuer/subject")
	}
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	userID, err := r.upsertUserByExternal(ctx, issuer, subject, display, emailPtr)
	if err != nil {
		return nil, fmt.Errorf("upsert attendee: %w", err)
	}
	if err := r.ensureAttendeeProfile(ctx, userID, verified && email != ""); err != nil {
		return nil, err
	}
	return r.buildUser(userID, issuer, subject, display, email, nil), nil
}

// ensureAttendeeProfile creates the profile with a fresh presenter code on
// first sign-in and keeps email_verified in step with the token.
func (r *Resolver) ensureAttendeeProfile(ctx context.Context, userID uuid.UUID, verified bool) error {
	for attempt := 0; attempt < 5; attempt++ {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO attendee_profiles (user_id, presenter_code, email_verified)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id) DO UPDATE SET email_verified = EXCLUDED.email_verified`,
			userID, codes.New(), verified)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "attendee_profiles_presenter_code_key" {
			continue
		}
		if err != nil {
			return fmt.Errorf("attendee profile: %w", err)
		}
		return nil
	}
	return errors.New("attendee profile: could not allocate a presenter code")
}
