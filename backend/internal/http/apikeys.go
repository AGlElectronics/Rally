package http

import (
	"net/http"
	"strings"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// apiKeyHandler manages the public schedule API keys (staff with
// schedule.publish). The public schedule endpoints require one of these
// keys; the Programme page uses the builtin one (PUBLIC_API_BUILTIN_KEY).
type apiKeyHandler struct {
	keys *events.KeyService
}

// requirePublicKey authenticates the public schedule endpoints. Keys ride
// in "Authorization: Bearer <key>", or in "?key=" because calendar
// subscriptions cannot send headers. Missing or wrong keys get 401 (the
// switch being off is a 404 before this middleware runs).
func requirePublicKey(keys *events.KeyService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := r.URL.Query().Get("key")
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				presented = strings.TrimPrefix(h, "Bearer ")
			}
			ok, err := keys.Verify(r.Context(), presented)
			if err != nil {
				httperror.Render(w, r, err)
				return
			}
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="public schedule API"`)
				httperror.Render(w, r, httperror.New(httperror.CodeUnauthorized,
					"a public API key is required (Authorization: Bearer <key>, or ?key= for calendar feeds)"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *apiKeyHandler) list(w http.ResponseWriter, r *http.Request) {
	keys, err := h.keys.List(r.Context())
	render(w, r, http.StatusOK, map[string]any{"items": keys}, err)
}

func (h *apiKeyHandler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	createdBy := ""
	if u, ok := auth.FromContext(r.Context()); ok {
		createdBy = u.DisplayName
	}
	key, token, err := h.keys.Create(r.Context(), in.Name, createdBy)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	// The token appears in this response only; the table stores its hash.
	writeJSON(w, http.StatusCreated, map[string]any{"key": key, "token": token})
}

func (h *apiKeyHandler) revoke(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "key")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if err := h.keys.Revoke(r.Context(), id); err != nil {
		httperror.Render(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
