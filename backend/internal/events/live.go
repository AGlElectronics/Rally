package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Live-day actions on the published schedule.
const (
	LiveDelay   = "delay"
	LiveCancel  = "cancel"
	LiveRestore = "restore"
	LiveMove    = "move"
)

// LiveInput is one live-day change. Delay uses Minutes (negative = earlier)
// and PushRoom; Move uses RoomID and/or StartsAt. Force applies a change
// that breaks a blocking rule (the lead decides on the day).
type LiveInput struct {
	Action   string     `json:"action"`
	Minutes  int        `json:"minutes"`
	PushRoom bool       `json:"push_room"`
	RoomID   *uuid.UUID `json:"room_id"`
	StartsAt *time.Time `json:"starts_at"`
	Force    bool       `json:"force"`
	// Announce drafts a Telegram announcement (default true).
	Announce *bool `json:"announce"`
}

// LiveResult is the new state after a live change.
type LiveResult struct {
	Schedule     *Schedule     `json:"schedule"`
	Version      int           `json:"version"`
	Moved        int           `json:"moved"`
	Conflicts    []Conflict    `json:"conflicts"`
	Announcement *Announcement `json:"announcement"`
}

// Announcement is a message for the Telegram chats; a person confirms it.
type Announcement struct {
	ID        uuid.UUID       `json:"id"`
	EditionID uuid.UUID       `json:"edition_id"`
	ItemID    *uuid.UUID      `json:"item_id"`
	Body      string          `json:"body"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result"`
	CreatedBy string          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
	SentBy    string          `json:"sent_by"`
	SentAt    *time.Time      `json:"sent_at"`
}

// Live applies a live-day change to the published schedule (a new version,
// so the public API shows it at once) and to the draft, and drafts an
// announcement. Requires schedule.publish.
func (s *ScheduleService) Live(ctx context.Context, editionID, itemID uuid.UUID, in LiveInput) (*LiveResult, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermSchedulePublish, orgID, editionID); err != nil {
		return nil, err
	}
	switch in.Action {
	case LiveDelay:
		if in.Minutes == 0 || in.Minutes < -720 || in.Minutes > 720 {
			return nil, httperror.Validation("invalid change", map[string]string{"minutes": "must be between -720 and 720, not 0"})
		}
	case LiveMove:
		if in.RoomID == nil && in.StartsAt == nil {
			return nil, httperror.Validation("invalid change", map[string]string{"room_id": "give a new room and/or start time"})
		}
	case LiveCancel, LiveRestore:
	default:
		return nil, httperror.Validation("invalid change", map[string]string{"action": "must be delay, cancel, restore or move"})
	}

	out := &LiveResult{}
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		st, _, err := lockSchedule(ctx, tx, orgID, editionID)
		if err != nil {
			return err
		}
		cur, err := readVersion(ctx, tx, editionID, 0)
		var he *httperror.Error
		if errors.As(err, &he) && he.Code == httperror.CodeNotFound {
			return httperror.Conflict("publish the schedule before making live changes")
		}
		if err != nil {
			return err
		}
		ed, err := getEdition(ctx, tx, editionID)
		if err != nil {
			return err
		}
		loc := editionLocation(ed)
		snap := cur.Data
		idx := -1
		for i := range snap.Items {
			if snap.Items[i].ID == itemID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return httperror.NotFound("published item")
		}
		target := snap.Items[idx]
		rooms := map[uuid.UUID]string{}
		for _, r := range snap.Rooms {
			rooms[r.ID] = r.Name
		}

		changed := map[uuid.UUID]bool{itemID: true}
		var body, note string
		switch in.Action {
		case LiveDelay:
			shift := time.Duration(in.Minutes) * time.Minute
			day := target.StartsAt.In(loc).Format("2006-01-02")
			for i := range snap.Items {
				it := &snap.Items[i]
				push := in.PushRoom && it.ID != itemID && it.RoomID == target.RoomID && it.Status == "scheduled" &&
					!it.StartsAt.Before(target.StartsAt) && it.StartsAt.In(loc).Format("2006-01-02") == day
				if it.ID == itemID || push {
					it.StartsAt, it.EndsAt = it.StartsAt.Add(shift), it.EndsAt.Add(shift)
					changed[it.ID] = true
				}
			}
			moved := snap.Items[idx]
			word := "delayed"
			mins := in.Minutes
			if mins < 0 {
				word, mins = "moved earlier", -mins
			}
			body = fmt.Sprintf("%s: %s in %s is %s by %d min and now runs %s\u2013%s.",
				capitalize(word), quote(moved.Title), rooms[moved.RoomID], word, mins,
				moved.StartsAt.In(loc).Format("15:04"), moved.EndsAt.In(loc).Format("15:04"))
			if len(changed) > 1 {
				body += fmt.Sprintf("\nThe rest of the day in %s moves by %d min too.", rooms[moved.RoomID], mins)
			}
			note = fmt.Sprintf("%s %s by %d min", target.Title, word, mins)
		case LiveCancel:
			if target.Status == "cancelled" {
				return httperror.Conflict("this item is already cancelled")
			}
			snap.Items[idx].Status = "cancelled"
			body = fmt.Sprintf("Cancelled: %s (%s, %s).", quote(target.Title), rooms[target.RoomID], target.StartsAt.In(loc).Format("Mon 15:04"))
			note = target.Title + " cancelled"
		case LiveRestore:
			if target.Status != "cancelled" {
				return httperror.Conflict("this item isn't cancelled")
			}
			snap.Items[idx].Status = "scheduled"
			body = fmt.Sprintf("Back on: %s is on again in %s at %s.", quote(target.Title), rooms[target.RoomID], target.StartsAt.In(loc).Format("Mon 15:04"))
			note = target.Title + " back on"
		case LiveMove:
			it := &snap.Items[idx]
			if in.RoomID != nil {
				if _, ok := rooms[*in.RoomID]; !ok {
					return httperror.Validation("invalid change", map[string]string{"room_id": "is not a published room"})
				}
				it.RoomID = *in.RoomID
			}
			if in.StartsAt != nil {
				length := it.EndsAt.Sub(it.StartsAt)
				it.StartsAt, it.EndsAt = *in.StartsAt, in.StartsAt.Add(length)
			}
			body = fmt.Sprintf("Moved: %s is now in %s at %s.", quote(it.Title), rooms[it.RoomID], it.StartsAt.In(loc).Format("Mon 15:04"))
			note = it.Title + " moved"
		}

		dbRooms, err := listRooms(ctx, tx, editionID)
		if err != nil {
			return err
		}
		types, err := listEventTypes(ctx, tx, editionID)
		if err != nil {
			return err
		}
		out.Conflicts = checkConflicts(toRuleItems(snap.Items), ruleWorldFor(st, dbRooms, types), changed)
		if hasBlocking(out.Conflicts) && !in.Force {
			return blockedError(out.Conflicts)
		}

		v, err := insertVersion(ctx, tx, orgID, editionID, snap, "live", note, u.ID)
		if err != nil {
			return err
		}
		out.Version, out.Moved = v, len(changed)
		// Mirror into the draft without marking it changed, so the live
		// change isn't reported as unpublished work.
		for _, it := range snap.Items {
			if !changed[it.ID] {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE schedule_items SET room_id=$2, starts_at=$3, ends_at=$4, status=$5, updated_at=now()
				WHERE id=$1 AND edition_id=$6`, it.ID, it.RoomID, it.StartsAt, it.EndsAt, it.Status, editionID); err != nil {
				return httperror.Internal(err)
			}
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "schedule.live."+in.Action, string(domain.ResourceScheduleItem),
			itemID, target, snap.Items[idx], note); err != nil {
			return err
		}
		if in.Announce == nil || *in.Announce {
			var draftItem *uuid.UUID
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schedule_items WHERE id=$1)`, itemID).Scan(&exists); err != nil {
				return httperror.Internal(err)
			}
			if exists {
				draftItem = &itemID
			}
			a, err := insertAnnouncement(ctx, tx, orgID, editionID, draftItem, body, u.ID)
			if err != nil {
				return err
			}
			out.Announcement = a
		}
		out.Schedule, err = s.load(ctx, tx, editionID)
		return err
	})
	if err != nil {
		return nil, wrapTxErr(err)
	}
	return out, nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

const announcementSelect = `SELECT a.id, a.edition_id, a.item_id, a.body, a.status, a.result,
	COALESCE(cu.display_name, ''), a.created_at, COALESCE(su.display_name, ''), a.sent_at
	FROM announcements a
	LEFT JOIN users cu ON cu.id = a.created_by
	LEFT JOIN users su ON su.id = a.sent_by`

func scanAnnouncement(row pgx.Row) (*Announcement, error) {
	var a Announcement
	err := row.Scan(&a.ID, &a.EditionID, &a.ItemID, &a.Body, &a.Status, &a.Result, &a.CreatedBy, &a.CreatedAt, &a.SentBy, &a.SentAt)
	switch {
	case err == nil:
		return &a, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("announcement")
	default:
		return nil, httperror.Internal(err)
	}
}

func insertAnnouncement(ctx context.Context, tx pgx.Tx, orgID, editionID uuid.UUID, itemID *uuid.UUID, body string, by uuid.UUID) (*Announcement, error) {
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO announcements (organization_id, edition_id, item_id, body, created_by)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`, orgID, editionID, itemID, body, by).Scan(&id); err != nil {
		return nil, httperror.Internal(err)
	}
	return scanAnnouncement(tx.QueryRow(ctx, announcementSelect+` WHERE a.id=$1`, id))
}

func checkAnnouncementBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", httperror.Validation("invalid announcement", map[string]string{"body": "is required"})
	}
	if len([]rune(body)) > 4000 {
		return "", httperror.Validation("invalid announcement", map[string]string{"body": "must be at most 4000 characters"})
	}
	return body, nil
}

// Announcements lists an edition's announcements, newest first.
func (s *ScheduleService) Announcements(ctx context.Context, editionID uuid.UUID) ([]Announcement, error) {
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, announcementSelect+` WHERE a.edition_id=$1 ORDER BY a.created_at DESC LIMIT 200`, editionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		a, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// CreateAnnouncement drafts a free-text announcement. Requires schedule.publish.
func (s *ScheduleService) CreateAnnouncement(ctx context.Context, editionID uuid.UUID, body string) (*Announcement, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermSchedulePublish, orgID, editionID); err != nil {
		return nil, err
	}
	body, err = checkAnnouncementBody(body)
	if err != nil {
		return nil, err
	}
	var out *Announcement
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		out, err = insertAnnouncement(ctx, tx, orgID, editionID, nil, body, u.ID)
		return err
	})
	return out, wrapTxErr(err)
}

func (s *ScheduleService) lockAnnouncement(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Announcement, uuid.UUID, error) {
	var orgID, editionID uuid.UUID
	var status string
	err := tx.QueryRow(ctx, `SELECT organization_id, edition_id, status FROM announcements WHERE id=$1 FOR UPDATE`, id).Scan(&orgID, &editionID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, uuid.Nil, httperror.NotFound("announcement")
	}
	if err != nil {
		return nil, uuid.Nil, httperror.Internal(err)
	}
	if err := auth.RequireForEdition(ctx, domain.PermSchedulePublish, orgID, editionID); err != nil {
		return nil, uuid.Nil, err
	}
	if status != "draft" {
		return nil, uuid.Nil, httperror.Conflict("this announcement was already " + status)
	}
	a, err := scanAnnouncement(tx.QueryRow(ctx, announcementSelect+` WHERE a.id=$1`, id))
	return a, orgID, err
}

// UpdateAnnouncement edits a draft. Requires schedule.publish.
func (s *ScheduleService) UpdateAnnouncement(ctx context.Context, id uuid.UUID, body string) (*Announcement, error) {
	body, err := checkAnnouncementBody(body)
	if err != nil {
		return nil, err
	}
	var out *Announcement
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, _, err := s.lockAnnouncement(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE announcements SET body=$2 WHERE id=$1`, id, body); err != nil {
			return httperror.Internal(err)
		}
		out, err = scanAnnouncement(tx.QueryRow(ctx, announcementSelect+` WHERE a.id=$1`, id))
		return err
	})
	return out, wrapTxErr(err)
}

// DiscardAnnouncement drops a draft. Requires schedule.publish.
func (s *ScheduleService) DiscardAnnouncement(ctx context.Context, id uuid.UUID) (*Announcement, error) {
	var out *Announcement
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, _, err := s.lockAnnouncement(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE announcements SET status='discarded' WHERE id=$1`, id); err != nil {
			return httperror.Internal(err)
		}
		var err error
		out, err = scanAnnouncement(tx.QueryRow(ctx, announcementSelect+` WHERE a.id=$1`, id))
		return err
	})
	return out, wrapTxErr(err)
}

type sendResult struct {
	Chat   string `json:"chat"`
	OK     bool   `json:"ok"`
	DryRun bool   `json:"dry_run,omitempty"`
	Error  string `json:"error,omitempty"`
}

// SendAnnouncement posts a draft to every enabled chat of the edition. It is
// marked sent when at least one chat got it. Requires schedule.publish.
func (s *ScheduleService) SendAnnouncement(ctx context.Context, id uuid.UUID) (*Announcement, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	var out *Announcement
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		a, orgID, err := s.lockAnnouncement(ctx, tx, id)
		if err != nil {
			return err
		}
		chats, err := listChats(ctx, tx, a.EditionID)
		if err != nil {
			return err
		}
		results := []sendResult{}
		sentAny := false
		for _, c := range chats {
			if !c.Enabled {
				continue
			}
			r := sendResult{Chat: c.Name, DryRun: !s.tg.Configured()}
			if err := s.tg.Send(ctx, c.ChatID, a.Body); err != nil {
				r.Error = err.Error()
			} else {
				r.OK, sentAny = true, true
			}
			results = append(results, r)
		}
		if len(results) == 0 {
			return httperror.Conflict("no Telegram chats are switched on for this edition")
		}
		raw, _ := json.Marshal(results)
		if !sentAny {
			if _, err := tx.Exec(ctx, `UPDATE announcements SET result=$2 WHERE id=$1`, id, raw); err != nil {
				return httperror.Internal(err)
			}
			out, err = scanAnnouncement(tx.QueryRow(ctx, announcementSelect+` WHERE a.id=$1`, id))
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE announcements SET status='sent', result=$2, sent_by=$3, sent_at=now() WHERE id=$1`,
			id, raw, u.ID); err != nil {
			return httperror.Internal(err)
		}
		if err := audit.Record(ctx, tx, orgID, &a.EditionID, "announcement.send", string(domain.ResourceAnnouncement),
			id, nil, map[string]any{"body": a.Body, "results": results}, ""); err != nil {
			return err
		}
		out, err = scanAnnouncement(tx.QueryRow(ctx, announcementSelect+` WHERE a.id=$1`, id))
		return err
	})
	return out, wrapTxErr(err)
}

// TelegramChat is a chat or channel the bot posts announcements to.
type TelegramChat struct {
	ID        uuid.UUID `json:"id"`
	EditionID uuid.UUID `json:"edition_id"`
	Name      string    `json:"name"`
	ChatID    string    `json:"chat_id"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// TelegramChatInput is the create/replace payload.
type TelegramChatInput struct {
	Name    string `json:"name"`
	ChatID  string `json:"chat_id"`
	Enabled bool   `json:"enabled"`
}

func (in *TelegramChatInput) normalize() error {
	in.Name, in.ChatID = strings.TrimSpace(in.Name), strings.TrimSpace(in.ChatID)
	issues := map[string]string{}
	if in.Name == "" {
		issues["name"] = "is required"
	}
	if in.ChatID == "" {
		issues["chat_id"] = "is required (a number like -1001234567890 or @channelname)"
	} else if len(in.ChatID) > 100 || strings.ContainsAny(in.ChatID, " \t\n") {
		issues["chat_id"] = "doesn't look like a Telegram chat id"
	}
	return validationIfAny("invalid chat", issues)
}

const chatColumns = `id, edition_id, name, chat_id, enabled, created_at`

func scanChat(row pgx.Row) (*TelegramChat, error) {
	var c TelegramChat
	err := row.Scan(&c.ID, &c.EditionID, &c.Name, &c.ChatID, &c.Enabled, &c.CreatedAt)
	switch {
	case err == nil:
		return &c, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("chat")
	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, httperror.Validation("invalid chat", map[string]string{"chat_id": "is already added"})
		}
		return nil, httperror.Internal(err)
	}
}

func listChats(ctx context.Context, q querier, editionID uuid.UUID) ([]TelegramChat, error) {
	rows, err := q.Query(ctx, `SELECT `+chatColumns+` FROM telegram_chats WHERE edition_id=$1 ORDER BY name`, editionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []TelegramChat{}
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// Chats lists the edition's Telegram chats.
func (s *ScheduleService) Chats(ctx context.Context, editionID uuid.UUID) ([]TelegramChat, error) {
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	return listChats(ctx, s.pool, editionID)
}

// CreateChat adds a chat. Requires schedule.publish.
func (s *ScheduleService) CreateChat(ctx context.Context, editionID uuid.UUID, in TelegramChatInput) (*TelegramChat, error) {
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermSchedulePublish, orgID, editionID); err != nil {
		return nil, err
	}
	if err := in.normalize(); err != nil {
		return nil, err
	}
	var out *TelegramChat
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		out, err = scanChat(tx.QueryRow(ctx, `INSERT INTO telegram_chats (organization_id, edition_id, name, chat_id, enabled)
			VALUES ($1,$2,$3,$4,$5) RETURNING `+chatColumns, orgID, editionID, in.Name, in.ChatID, in.Enabled))
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, orgID, &editionID, "telegram_chat.create", string(domain.ResourceChat), out.ID, nil, out, "")
	})
	return out, wrapTxErr(err)
}

func (s *ScheduleService) lockChat(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*TelegramChat, uuid.UUID, error) {
	var orgID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT organization_id FROM telegram_chats WHERE id=$1`, id).Scan(&orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, uuid.Nil, httperror.NotFound("chat")
		}
		return nil, uuid.Nil, httperror.Internal(err)
	}
	c, err := scanChat(tx.QueryRow(ctx, `SELECT `+chatColumns+` FROM telegram_chats WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return nil, uuid.Nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermSchedulePublish, orgID, c.EditionID); err != nil {
		return nil, uuid.Nil, err
	}
	return c, orgID, nil
}

// UpdateChat replaces a chat. Requires schedule.publish.
func (s *ScheduleService) UpdateChat(ctx context.Context, id uuid.UUID, in TelegramChatInput) (*TelegramChat, error) {
	if err := in.normalize(); err != nil {
		return nil, err
	}
	var out *TelegramChat
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, orgID, err := s.lockChat(ctx, tx, id)
		if err != nil {
			return err
		}
		out, err = scanChat(tx.QueryRow(ctx, `UPDATE telegram_chats SET name=$2, chat_id=$3, enabled=$4 WHERE id=$1
			RETURNING `+chatColumns, id, in.Name, in.ChatID, in.Enabled))
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, orgID, &before.EditionID, "telegram_chat.update", string(domain.ResourceChat), id, before, out, "")
	})
	return out, wrapTxErr(err)
}

// DeleteChat removes a chat. Requires schedule.publish.
func (s *ScheduleService) DeleteChat(ctx context.Context, id uuid.UUID) error {
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, orgID, err := s.lockChat(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM telegram_chats WHERE id=$1`, id); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, orgID, &before.EditionID, "telegram_chat.delete", string(domain.ResourceChat), id, before, nil, "")
	})
	return wrapTxErr(err)
}
