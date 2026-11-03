package http

import (
	"net/http"

	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// editionHandler serves the read-only edition list the SPA needs to pick a
// CfP context.
type editionHandler struct {
	svc *events.EditionService
}

func (h *editionHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListVisible(r.Context())
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}
