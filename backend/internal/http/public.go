package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// publicHandler serves the attendee-facing submission site API.
type publicHandler struct {
	public *events.PublicService
	subs   *events.SubmissionService
}

func (h *publicHandler) calls(w http.ResponseWriter, r *http.Request) {
	items, err := h.public.OpenCalls(r.Context())
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *publicHandler) call(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "cfp")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	c, err := h.public.Call(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *publicHandler) me(w http.ResponseWriter, r *http.Request) {
	p, err := h.subs.Profile(r.Context())
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *publicHandler) mine(w http.ResponseWriter, r *http.Request) {
	items, err := h.subs.ListMine(r.Context())
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	u, _ := auth.FromContext(r.Context())
	for i := range items {
		events.RedactFor(u, &items[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// writeSubmission redacts owner-only data before answering.
func writeSubmission(w http.ResponseWriter, r *http.Request, status int, s *events.Submission, err error) {
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if u, ok := auth.FromContext(r.Context()); ok {
		events.RedactFor(u, s)
	}
	writeJSON(w, status, s)
}

func (h *publicHandler) addPerson(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.AddPersonInput
	if err := decodeJSON(w, r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.subs.AddPerson(r.Context(), id, in)
	writeSubmission(w, r, http.StatusOK, s, err)
}

func (h *publicHandler) removePerson(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	pid, err := parseUUIDParam(r, "person")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.subs.RemovePerson(r.Context(), id, pid)
	writeSubmission(w, r, http.StatusOK, s, err)
}

func (h *publicHandler) reissueInvite(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	pid, err := parseUUIDParam(r, "person")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	s, err := h.subs.ReissueInvite(r.Context(), id, pid)
	writeSubmission(w, r, http.StatusOK, s, err)
}

func (h *publicHandler) leave(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "submission")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if err := h.subs.Leave(r.Context(), id); err != nil {
		httperror.Render(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *publicHandler) invite(w http.ResponseWriter, r *http.Request) {
	p, err := h.subs.PreviewInvite(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *publicHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	s, err := h.subs.AcceptInvite(r.Context(), chi.URLParam(r, "code"))
	writeSubmission(w, r, http.StatusOK, s, err)
}

func (h *publicHandler) declineInvite(w http.ResponseWriter, r *http.Request) {
	if err := h.subs.DeclineInvite(r.Context(), chi.URLParam(r, "code")); err != nil {
		httperror.Render(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fileHandler serves upload slots and download links (staff and public).
type fileHandler struct {
	svc *events.FileService
}

func (h *fileHandler) start(w http.ResponseWriter, r *http.Request) {
	var in events.UploadInput
	if err := decodeJSON(w, r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	t, err := h.svc.StartUpload(r.Context(), in)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *fileHandler) complete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "file")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	f, err := h.svc.CompleteUpload(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *fileHandler) download(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "file")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	url, err := h.svc.DownloadURL(r.Context(), id)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}
