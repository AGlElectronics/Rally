package http

import (
	"net/http"

	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// tagsHandler serves the organisation tag pool (propose / approve / reject).
type tagsHandler struct {
	svc *events.TagService
}

func (h *tagsHandler) listForEdition(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	f := events.TagFilter{
		Status: r.URL.Query().Get("status"),
		Query:  r.URL.Query().Get("q"),
	}
	// Autocomplete (?q=) defaults to approved tags.
	if f.Status == "" && f.Query != "" {
		f.Status = events.TagApproved
	}
	items, err := h.svc.ListForEdition(r.Context(), editionID, f)
	render(w, r, http.StatusOK, map[string]any{"items": items}, err)
}

func (h *tagsHandler) propose(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.TagInput
	if err := decodeJSON(w, r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	t, err := h.svc.Propose(r.Context(), editionID, in)
	render(w, r, http.StatusCreated, t, err)
}

func (h *tagsHandler) approve(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "tag")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = decodeOptionalJSON(w, r, &body)
	t, err := h.svc.Approve(r.Context(), id, body.Note)
	render(w, r, http.StatusOK, t, err)
}

func (h *tagsHandler) reject(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "tag")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = decodeOptionalJSON(w, r, &body)
	t, err := h.svc.Reject(r.Context(), id, body.Note)
	render(w, r, http.StatusOK, t, err)
}

func (h *tagsHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "tag")
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

// Ensure PermTagApprove is referenced so unused-import checkers stay quiet
// if middleware wires it from the router only.
var _ = domain.PermTagApprove
