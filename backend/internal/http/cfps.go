package http

import (
	"net/http"

	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// cfpHandler serves call-for-papers endpoints:
//
//	GET    /api/v1/editions/{edition}/cfps
//	POST   /api/v1/editions/{edition}/cfps
//	GET    /api/v1/cfps/{cfp}
//	PUT    /api/v1/cfps/{cfp}
//	DELETE /api/v1/cfps/{cfp}
type cfpHandler struct {
	svc *events.CfPService
}

func (h *cfpHandler) list(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	items, err := h.svc.ListForEdition(r.Context(), editionID)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *cfpHandler) create(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.CfPInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	c, err := h.svc.Create(r.Context(), editionID, in)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *cfpHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "cfp")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	c, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *cfpHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "cfp")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.CfPInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	c, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *cfpHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "cfp")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httperror.Render(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
