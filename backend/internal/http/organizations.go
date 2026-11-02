package http

import (
	"net/http"

	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// organizationHandler groups the organization-management endpoints wired
// into the router. Only the read path is implemented in the scaffold; the
// create/update actions arrive with the first vertical slice.
type organizationHandler struct {
	svc *events.OrganizationService
}

func (h *organizationHandler) list(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.svc.ListVisible(r.Context())
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	// Wrap the slice in an object so we can add pagination metadata later
	// without a breaking API change.
	writeJSON(w, http.StatusOK, map[string]any{
		"items": orgs,
		"total": len(orgs),
	})
}

func (h *organizationHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "organization")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	org, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, org)
}
