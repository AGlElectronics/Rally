package auth

import (
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Middleware authenticates each request based on cfg.AuthMode:
//
//   - AuthModeDev reads X-Dev-User (or DEV_DEFAULT_USER) and provisions a
//     local user under the reserved "dev" issuer. Honoured only with
//     APP_ENV=development; otherwise every request is refused.
//   - AuthModeOIDC verifies the Authorization: Bearer <JWT>, then
//     provisions/refreshes the user from (issuer, subject).
//
// On any failure the request is terminated with a typed JSON error; no
// handler further down the chain sees an unauthenticated request.
func Middleware(cfg *config.Config, resolver *Resolver, oidcVerifier *OIDCVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			switch {
			case cfg.DevAuth():
				header := r.Header.Get("X-Dev-User")
				if header == "" {
					header = cfg.DevDefaultUser
				}
				user, err := resolver.LoadOrProvisionDevUser(ctx, header)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("dev auth failed")
					httperror.Render(w, r, httperror.Internal(err))
					return
				}
				ctx = WithUser(ctx, user)

			case cfg.AuthMode == config.AuthModeOIDC:
				if oidcVerifier == nil {
					httperror.Render(w, r, httperror.Internal(nil))
					return
				}
				bearer := bearerFromHeader(r.Header.Get("Authorization"))
				if bearer == "" {
					httperror.Render(w, r, httperror.New(httperror.CodeUnauthorized, "missing bearer token"))
					return
				}
				claims, err := oidcVerifier.Verify(ctx, bearer)
				if err != nil {
					log.Ctx(ctx).Debug().Err(err).Msg("oidc verify failed")
					httperror.Render(w, r, httperror.New(httperror.CodeUnauthorized, "invalid token"))
					return
				}
				user, err := resolver.LoadOrProvisionOIDCUser(ctx, claims)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("oidc resolver failed")
					httperror.Render(w, r, httperror.Internal(err))
					return
				}
				ctx = WithUser(ctx, user)

			default:
				httperror.Render(w, r, httperror.Internal(nil))
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerFromHeader(h string) string {
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
