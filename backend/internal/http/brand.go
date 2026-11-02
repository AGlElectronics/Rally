package http

import (
	"net/http"

	"github.com/AGlElectronics/rally/backend/internal/brand"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// brandHandler serves the active brand package. The SPA boots once per
// page load and caches the result; the /api/v1/brand endpoint is PUBLIC
// (unauthenticated) because the login screen needs product name & colors
// before the user signs in.
type brandHandler struct {
	provider brand.Provider
}

func (h *brandHandler) get(w http.ResponseWriter, r *http.Request) {
	b, err := h.provider.Get(r.Context())
	if err != nil {
		httperror.Render(w, r, httperror.Internal(err))
		return
	}
	writeJSON(w, http.StatusOK, b)
}
