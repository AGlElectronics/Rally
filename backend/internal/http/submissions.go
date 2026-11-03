package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// submissionHandler serves CfP submission endpoints.
type submissionHandler struct {
	svc *events.SubmissionService
}

func (h *submissionHandler) listForEdition(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	f := events.SubmissionFilter{Mine: r.URL.Query().Get("mine") == "1"}
	if state := r.URL.Query().Get("state"); state != "" {
		f.State = domain.SubmissionState(state)
	}
	if raw := r.URL.Query().Get("cfp_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httperror.Render(w, r, httperror.BadRequest("invalid cfp_id"))
			return
		}
		f.CfPID = &id
	}
	items, err := h.svc.List(r.Context(), editionID, f)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *submissionHandler) create(w http.ResponseWriter, r *http.Request) {
	cfpID, err := parseUUIDParam(r, "cfp")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.SubmissionInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.svc.Create(r.Context(), cfpID, in)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *submissionHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *submissionHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var p events.SubmissionPatch
	if err := decodeJSON(r, &p); err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.svc.Update(r.Context(), id, p)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *submissionHandler) submit(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.svc.Submit(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *submissionHandler) withdraw(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.svc.Withdraw(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

type decisionInput struct {
	Note string `json:"note"`
}

func (h *submissionHandler) accept(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in decisionInput
	_ = decodeJSON(r, &in)
	s, err := h.svc.Accept(r.Context(), id, in.Note)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *submissionHandler) reject(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in decisionInput
	_ = decodeJSON(r, &in)
	s, err := h.svc.Reject(r.Context(), id, in.Note)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}
