package http

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AGlElectronics/rally/backend/internal/denlink"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// PublicScheduleSetting is the Den organisation setting that switches the
// public schedule API on or off.
const PublicScheduleSetting = "events_public_schedule"

// publicScheduleSwitch returns the effective on/off state: Den's setting
// when paired and set, else the module's own config.
func publicScheduleSwitch(den *denlink.Client, fallback bool) func() bool {
	return func() bool {
		if den != nil {
			if b := den.Bundle(); b != nil {
				if v, ok := b.Settings.Data[PublicScheduleSetting].(bool); ok {
					return v
				}
			}
		}
		return fallback
	}
}

// scheduleHandler serves rooms, the schedule builder, publishing, live
// changes, announcements and Telegram chats.
type scheduleHandler struct {
	rooms *events.RoomService
	svc   *events.ScheduleService
	// productName labels the iCal feed.
	productName func() string
}

func render(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (h *scheduleHandler) listRooms(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	items, err := h.rooms.List(r.Context(), id)
	render(w, r, http.StatusOK, map[string]any{"items": items}, err)
}

func (h *scheduleHandler) createRoom(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.RoomInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	room, err := h.rooms.Create(r.Context(), id, in)
	render(w, r, http.StatusCreated, room, err)
}

func (h *scheduleHandler) updateRoom(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "room")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.RoomInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	room, err := h.rooms.Update(r.Context(), id, in)
	render(w, r, http.StatusOK, room, err)
}

func (h *scheduleHandler) deleteRoom(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "room")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if err := h.rooms.Delete(r.Context(), id); err != nil {
		httperror.Render(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *scheduleHandler) updateDates(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.EditionDatesInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	ed, err := h.rooms.UpdateEditionDates(r.Context(), id, in)
	render(w, r, http.StatusOK, ed, err)
}

func (h *scheduleHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	sc, err := h.svc.Get(r.Context(), id)
	render(w, r, http.StatusOK, sc, err)
}

func (h *scheduleHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.ScheduleSettings
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	sc, err := h.svc.UpdateSettings(r.Context(), id, in)
	render(w, r, http.StatusOK, sc, err)
}

func (h *scheduleHandler) createItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.ItemInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	sc, err := h.svc.CreateItem(r.Context(), id, in)
	render(w, r, http.StatusCreated, sc, err)
}

func (h *scheduleHandler) updateItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "item")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.ItemInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	sc, err := h.svc.UpdateItem(r.Context(), id, in)
	render(w, r, http.StatusOK, sc, err)
}

func (h *scheduleHandler) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "item")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	sc, err := h.svc.DeleteItem(r.Context(), id)
	render(w, r, http.StatusOK, sc, err)
}

type publishInput struct {
	Note string `json:"note"`
}

func (h *scheduleHandler) publish(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in publishInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	sc, err := h.svc.Publish(r.Context(), id, in.Note)
	render(w, r, http.StatusOK, sc, err)
}

func (h *scheduleHandler) versions(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	items, err := h.svc.Versions(r.Context(), id)
	render(w, r, http.StatusOK, map[string]any{"items": items}, err)
}

func versionParam(r *http.Request) (int, error) {
	v, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || v < 1 {
		return 0, httperror.BadRequest("invalid version")
	}
	return v, nil
}

func (h *scheduleHandler) version(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	v, err := versionParam(r)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	out, err := h.svc.Version(r.Context(), id, v)
	render(w, r, http.StatusOK, out, err)
}

func (h *scheduleHandler) restore(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	v, err := versionParam(r)
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	out, err := h.svc.Restore(r.Context(), id, v)
	render(w, r, http.StatusOK, out, err)
}

func (h *scheduleHandler) live(w http.ResponseWriter, r *http.Request) {
	editionID, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	itemID, err := parseUUIDParam(r, "item")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.LiveInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	out, err := h.svc.Live(r.Context(), editionID, itemID, in)
	render(w, r, http.StatusOK, out, err)
}

func (h *scheduleHandler) announcements(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	items, err := h.svc.Announcements(r.Context(), id)
	render(w, r, http.StatusOK, map[string]any{"items": items}, err)
}

type announcementInput struct {
	Body string `json:"body"`
}

func (h *scheduleHandler) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in announcementInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	a, err := h.svc.CreateAnnouncement(r.Context(), id, in.Body)
	render(w, r, http.StatusCreated, a, err)
}

func (h *scheduleHandler) updateAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "announcement")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in announcementInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	a, err := h.svc.UpdateAnnouncement(r.Context(), id, in.Body)
	render(w, r, http.StatusOK, a, err)
}

func (h *scheduleHandler) sendAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "announcement")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	a, err := h.svc.SendAnnouncement(r.Context(), id)
	render(w, r, http.StatusOK, a, err)
}

func (h *scheduleHandler) discardAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "announcement")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	a, err := h.svc.DiscardAnnouncement(r.Context(), id)
	render(w, r, http.StatusOK, a, err)
}

func (h *scheduleHandler) chats(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	items, err := h.svc.Chats(r.Context(), id)
	render(w, r, http.StatusOK, map[string]any{"items": items}, err)
}

func (h *scheduleHandler) createChat(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "edition")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.TelegramChatInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	c, err := h.svc.CreateChat(r.Context(), id, in)
	render(w, r, http.StatusCreated, c, err)
}

func (h *scheduleHandler) updateChat(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "chat")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	var in events.TelegramChatInput
	if err := decodeJSON(r, &in); err != nil {
		httperror.Render(w, r, err)
		return
	}
	c, err := h.svc.UpdateChat(r.Context(), id, in)
	render(w, r, http.StatusOK, c, err)
}

func (h *scheduleHandler) deleteChat(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "chat")
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if err := h.svc.DeleteChat(r.Context(), id); err != nil {
		httperror.Render(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requirePublicSchedule hides the public schedule endpoints when switched off.
func (h *scheduleHandler) requirePublicSchedule(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.svc.PublicEnabled() {
			httperror.Render(w, r, httperror.NotFound("public schedule"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *scheduleHandler) publicList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.PublicList(r.Context())
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	// private: the responses are authenticated by API key now, so shared
	// caches must not serve them to other clients.
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Header().Set("Vary", "Authorization")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// cacheable sets an ETag per published version and answers 304 when the
// website already has it.
func cacheable(w http.ResponseWriter, r *http.Request, p *events.PublicSchedule, suffix string) bool {
	etag := fmt.Sprintf(`"%s-v%d%s"`, p.Edition.ID, p.Version, suffix)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Header().Set("Vary", "Authorization")
	for _, tag := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		if strings.TrimSpace(tag) == etag {
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}
	return false
}

func (h *scheduleHandler) publicSchedule(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Public(r.Context(), chi.URLParam(r, "edition"))
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if cacheable(w, r, p, "") {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *scheduleHandler) publicICS(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Public(r.Context(), chi.URLParam(r, "edition"))
	if err != nil {
		httperror.Render(w, r, err)
		return
	}
	if cacheable(w, r, p, "-ics") {
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="`+p.Edition.Slug+`.ics"`)
	_, _ = w.Write([]byte(events.ICS(p, h.productName())))
}
