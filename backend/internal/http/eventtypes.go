package http

import (
	"net/http"

	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// eventTypeHandler serves edition event types.
type eventTypeHandler struct {
	svc *events.EventTypeService
}

func (h *eventTypeHandler) list(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	items, err := h.svc.List(r.Context(), editionID)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *eventTypeHandler) create(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.EventTypeInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	t, err := h.svc.Create(r.Context(), editionID, in)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *eventTypeHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "eventType")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.EventTypeInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	t, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *eventTypeHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "eventType")
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
