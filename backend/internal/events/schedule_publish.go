package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Snapshot is a frozen published schedule. It keeps staff-only items and
// people's user ids for staff and live checks; Public strips both.
type Snapshot struct {
	Edition struct {
		ID       uuid.UUID  `json:"id"`
		Slug     string     `json:"slug"`
		Title    string     `json:"title"`
		Timezone string     `json:"timezone"`
		StartAt  *time.Time `json:"start_at"`
		EndAt    *time.Time `json:"end_at"`
	} `json:"edition"`
	Rooms      []SnapshotRoom `json:"rooms"`
	EventTypes []SnapshotType `json:"event_types"`
	Items      []ScheduleItem `json:"items"`
}

// SnapshotRoom is a room as published.
type SnapshotRoom struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Capacity *int      `json:"capacity"`
}

// SnapshotType is an event type as published.
type SnapshotType struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

// VersionSummary lists a published version without its data.
type VersionSummary struct {
	Version     int       `json:"version"`
	Kind        string    `json:"kind"`
	Note        string    `json:"note"`
	PublishedAt time.Time `json:"published_at"`
	PublishedBy string    `json:"published_by"`
	ItemCount   int       `json:"item_count"`
	IsCurrent   bool      `json:"is_current"`
}

// Version is one published version with its data.
type Version struct {
	VersionSummary
	Data Snapshot `json:"data"`
}

func buildSnapshot(sc *Schedule) Snapshot {
	var snap Snapshot
	snap.Edition.ID = sc.Edition.ID
	snap.Edition.Slug = sc.Edition.Slug
	snap.Edition.Title = sc.Edition.Title
	snap.Edition.Timezone = sc.Edition.Timezone
	snap.Edition.StartAt = sc.Edition.StartAt
	snap.Edition.EndAt = sc.Edition.EndAt
	snap.Rooms = make([]SnapshotRoom, 0, len(sc.Rooms))
	for _, r := range sc.Rooms {
		snap.Rooms = append(snap.Rooms, SnapshotRoom{ID: r.ID, Name: r.Name, Capacity: r.Capacity})
	}
	snap.EventTypes = make([]SnapshotType, 0, len(sc.EventTypes))
	for _, t := range sc.EventTypes {
		snap.EventTypes = append(snap.EventTypes, SnapshotType{ID: t.ID, Name: t.Name, Color: t.Color})
	}
	snap.Items = sc.Items
	return snap
}

func insertVersion(ctx context.Context, tx pgx.Tx, orgID, editionID uuid.UUID, snap Snapshot, kind, note string, by uuid.UUID) (int, error) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return 0, httperror.Internal(err)
	}
	var next int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version), 0) + 1 FROM schedule_versions WHERE edition_id=$1`, editionID).Scan(&next); err != nil {
		return 0, httperror.Internal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schedule_versions (organization_id, edition_id, version, data, note, kind, published_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, orgID, editionID, next, raw, note, kind, by); err != nil {
		return 0, httperror.Internal(err)
	}
	return next, nil
}

// Publish freezes the draft as the new public version. Requires
// schedule.publish; refused while blocking conflicts exist.
func (s *ScheduleService) Publish(ctx context.Context, editionID uuid.UUID, note string) (*Schedule, error) {
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
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 500 {
		return nil, httperror.Validation("invalid publish", map[string]string{"note": "must be at most 500 characters"})
	}
	var out *Schedule
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, _, err := lockSchedule(ctx, tx, orgID, editionID); err != nil {
			return err
		}
		sc, err := s.load(ctx, tx, editionID)
		if err != nil {
			return err
		}
		if hasBlocking(sc.Conflicts) {
			return httperror.Conflict("fix the blocking conflicts before publishing").
				WithDetails(map[string]any{"conflicts": sc.Conflicts})
		}
		v, err := insertVersion(ctx, tx, orgID, editionID, buildSnapshot(sc), "publish", note, u.ID)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "schedule.publish", string(domain.ResourceSchedule),
			editionID, nil, map[string]any{"version": v, "items": len(sc.Items)}, note); err != nil {
			return err
		}
		out, err = s.load(ctx, tx, editionID)
		return err
	})
	return out, wrapTxErr(err)
}

// Versions lists published versions, newest first.
func (s *ScheduleService) Versions(ctx context.Context, editionID uuid.UUID) ([]VersionSummary, error) {
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT v.version, v.kind, v.note, v.published_at, COALESCE(u.display_name, ''),
			jsonb_array_length(COALESCE(v.data->'items', '[]'::jsonb))
		FROM schedule_versions v LEFT JOIN users u ON u.id = v.published_by
		WHERE v.edition_id=$1 ORDER BY v.version DESC`, editionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []VersionSummary{}
	for rows.Next() {
		var v VersionSummary
		if err := rows.Scan(&v.Version, &v.Kind, &v.Note, &v.PublishedAt, &v.PublishedBy, &v.ItemCount); err != nil {
			return nil, httperror.Internal(err)
		}
		v.IsCurrent = len(out) == 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func readVersion(ctx context.Context, q querier, editionID uuid.UUID, version int) (*Version, error) {
	var v Version
	var raw []byte
	sql := `SELECT v.version, v.kind, v.note, v.published_at, COALESCE(u.display_name, ''), v.data
		FROM schedule_versions v LEFT JOIN users u ON u.id = v.published_by WHERE v.edition_id=$1`
	args := []any{editionID}
	if version > 0 {
		sql += ` AND v.version=$2`
		args = append(args, version)
	} else {
		sql += ` ORDER BY v.version DESC LIMIT 1`
	}
	err := q.QueryRow(ctx, sql, args...).Scan(&v.Version, &v.Kind, &v.Note, &v.PublishedAt, &v.PublishedBy, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httperror.NotFound("published schedule")
	}
	if err != nil {
		return nil, httperror.Internal(err)
	}
	if err := json.Unmarshal(raw, &v.Data); err != nil {
		return nil, httperror.Internal(err)
	}
	v.ItemCount = len(v.Data.Items)
	return &v, nil
}

// Version returns one published version (0 = the current one).
func (s *ScheduleService) Version(ctx context.Context, editionID uuid.UUID, version int) (*Version, error) {
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	return readVersion(ctx, s.pool, editionID, version)
}

// RestoreResult reports what loading an old version into the draft skipped.
type RestoreResult struct {
	Schedule *Schedule `json:"schedule"`
	Skipped  []string  `json:"skipped"`
}

// Restore replaces the draft with a published version so it can be checked
// and published again. Items whose room was deleted since are skipped.
// Requires schedule.edit.
func (s *ScheduleService) Restore(ctx context.Context, editionID uuid.UUID, version int) (*RestoreResult, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, orgID, editionID); err != nil {
		return nil, err
	}
	out := &RestoreResult{Skipped: []string{}}
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, _, err := lockSchedule(ctx, tx, orgID, editionID); err != nil {
			return err
		}
		v, err := readVersion(ctx, tx, editionID, version)
		if err != nil {
			return err
		}
		rooms, err := listRooms(ctx, tx, editionID)
		if err != nil {
			return err
		}
		types, err := listEventTypes(ctx, tx, editionID)
		if err != nil {
			return err
		}
		roomOK, typeOK := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
		for _, r := range rooms {
			roomOK[r.ID] = true
		}
		for _, t := range types {
			typeOK[t.ID] = true
		}
		if _, err := tx.Exec(ctx, `UPDATE announcements SET item_id=NULL WHERE edition_id=$1`, editionID); err != nil {
			return httperror.Internal(err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schedule_items WHERE edition_id=$1`, editionID); err != nil {
			return httperror.Internal(err)
		}
		for _, it := range v.Data.Items {
			if !roomOK[it.RoomID] {
				out.Skipped = append(out.Skipped, it.Title+" (its room was deleted)")
				continue
			}
			if it.SubmissionID != nil {
				var state string
				err := tx.QueryRow(ctx, `SELECT state FROM submissions WHERE id=$1 AND edition_id=$2`, *it.SubmissionID, editionID).Scan(&state)
				if err != nil || state != "accepted" {
					out.Skipped = append(out.Skipped, it.Title+" (no longer accepted)")
					continue
				}
			}
			typeID := it.EventTypeID
			if typeID != nil && !typeOK[*typeID] {
				typeID = nil
			}
			if _, err := tx.Exec(ctx, `INSERT INTO schedule_items
				(id, organization_id, edition_id, submission_id, room_id, event_type_id, title, description,
				 starts_at, ends_at, staff_only, status, created_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
				it.ID, orgID, editionID, it.SubmissionID, it.RoomID, typeID, it.Title, it.Description,
				it.StartsAt, it.EndsAt, it.StaffOnly, it.Status, u.ID); err != nil {
				return httperror.Internal(err)
			}
		}
		if err := touchDraft(ctx, tx, editionID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "schedule.restore", string(domain.ResourceSchedule),
			editionID, nil, map[string]any{"version": v.Version, "skipped": out.Skipped}, ""); err != nil {
			return err
		}
		out.Schedule, err = s.load(ctx, tx, editionID)
		return err
	})
	if err != nil {
		return nil, wrapTxErr(err)
	}
	return out, nil
}

// PublicItem is an item as the website sees it.
type PublicItem struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Day         string     `json:"day"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	RoomID      uuid.UUID  `json:"room_id"`
	Room        string     `json:"room"`
	EventTypeID *uuid.UUID `json:"event_type_id"`
	EventType   string     `json:"event_type"`
	Color       string     `json:"color"`
	Status      string     `json:"status"`
	People      []string   `json:"people"`
}

// PublicSchedule is the read-only programme served to the website.
type PublicSchedule struct {
	Edition struct {
		ID       uuid.UUID `json:"id"`
		Slug     string    `json:"slug"`
		Title    string    `json:"title"`
		Timezone string    `json:"timezone"`
	} `json:"edition"`
	Version     int            `json:"version"`
	PublishedAt time.Time      `json:"published_at"`
	Days        []string       `json:"days"`
	Rooms       []SnapshotRoom `json:"rooms"`
	EventTypes  []SnapshotType `json:"event_types"`
	Items       []PublicItem   `json:"items"`
}

// PublicListing is one edition with a published schedule.
type PublicListing struct {
	EditionID   uuid.UUID `json:"edition_id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Version     int       `json:"version"`
	PublishedAt time.Time `json:"published_at"`
}

// PublicList returns every edition with a published schedule.
func (s *ScheduleService) PublicList(ctx context.Context) ([]PublicListing, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (v.edition_id) v.edition_id, e.slug, e.title, v.version, v.published_at
		FROM schedule_versions v JOIN editions e ON e.id = v.edition_id
		ORDER BY v.edition_id, v.version DESC`)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []PublicListing{}
	for rows.Next() {
		var l PublicListing
		if err := rows.Scan(&l.EditionID, &l.Slug, &l.Title, &l.Version, &l.PublishedAt); err != nil {
			return nil, httperror.Internal(err)
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, rows.Err()
}

// Public returns the current published schedule without staff-only items.
// Public serves the latest published version. ref is the edition id or, when
// only one edition with that slug has a published schedule, its slug.
func (s *ScheduleService) Public(ctx context.Context, ref string) (*PublicSchedule, error) {
	editionID, err := uuid.Parse(ref)
	if err != nil {
		rows, err := s.pool.Query(ctx, `SELECT DISTINCT e.id FROM editions e
			JOIN schedule_versions v ON v.edition_id = e.id WHERE e.slug = $1 LIMIT 2`, ref)
		if err != nil {
			return nil, httperror.Internal(err)
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return nil, httperror.Internal(err)
		}
		switch len(ids) {
		case 0:
			return nil, httperror.NotFound("published schedule")
		case 1:
			editionID = ids[0]
		default:
			return nil, httperror.Conflict("several editions use this slug; use the edition id")
		}
	}
	v, err := readVersion(ctx, s.pool, editionID, 0)
	if err != nil {
		return nil, err
	}
	return toPublic(v), nil
}

func toPublic(v *Version) *PublicSchedule {
	d := v.Data
	loc, err := time.LoadLocation(d.Edition.Timezone)
	if err != nil {
		loc = time.UTC
	}
	out := &PublicSchedule{Version: v.Version, PublishedAt: v.PublishedAt, Rooms: d.Rooms, EventTypes: d.EventTypes, Items: []PublicItem{}}
	out.Edition.ID, out.Edition.Slug, out.Edition.Title, out.Edition.Timezone = d.Edition.ID, d.Edition.Slug, d.Edition.Title, d.Edition.Timezone
	rooms := map[uuid.UUID]string{}
	for _, r := range d.Rooms {
		rooms[r.ID] = r.Name
	}
	types := map[uuid.UUID]SnapshotType{}
	for _, t := range d.EventTypes {
		types[t.ID] = t
	}
	days := map[string]bool{}
	if d.Edition.StartAt != nil && d.Edition.EndAt != nil {
		for day := d.Edition.StartAt.In(loc); !day.After(d.Edition.EndAt.In(loc)); day = day.AddDate(0, 0, 1) {
			days[day.Format("2006-01-02")] = true
		}
	}
	for _, it := range d.Items {
		if it.StaffOnly {
			continue
		}
		pi := PublicItem{ID: it.ID, Title: it.Title, Description: it.Description, StartsAt: it.StartsAt, EndsAt: it.EndsAt,
			Day: it.StartsAt.In(loc).Format("2006-01-02"), RoomID: it.RoomID, Room: rooms[it.RoomID],
			EventTypeID: it.EventTypeID, Status: it.Status, People: []string{}}
		if it.EventTypeID != nil {
			t := types[*it.EventTypeID]
			pi.EventType, pi.Color = t.Name, t.Color
		}
		for _, p := range it.People {
			pi.People = append(pi.People, p.Name)
		}
		days[pi.Day] = true
		out.Items = append(out.Items, pi)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		if !out.Items[i].StartsAt.Equal(out.Items[j].StartsAt) {
			return out.Items[i].StartsAt.Before(out.Items[j].StartsAt)
		}
		return out.Items[i].Room < out.Items[j].Room
	})
	out.Days = make([]string, 0, len(days))
	for day := range days {
		out.Days = append(out.Days, day)
	}
	sort.Strings(out.Days)
	return out
}

// ICS renders a published schedule as an iCalendar feed.
func ICS(p *PublicSchedule, productName string) string {
	var b strings.Builder
	line := func(s string) {
		// Fold at 75 octets as RFC 5545 asks, without splitting a rune.
		for len(s) > 75 {
			cut := 75
			for cut > 0 && (s[cut]&0xC0) == 0x80 {
				cut--
			}
			b.WriteString(s[:cut] + "\r\n")
			s = " " + s[cut:]
		}
		b.WriteString(s + "\r\n")
	}
	stamp := p.PublishedAt.UTC().Format("20060102T150405Z")
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//" + icsText(productName) + "//Rally//EN")
	line("CALSCALE:GREGORIAN")
	line("X-WR-CALNAME:" + icsText(p.Edition.Title))
	for _, it := range p.Items {
		line("BEGIN:VEVENT")
		line("UID:" + it.ID.String() + "@rally")
		line("DTSTAMP:" + stamp)
		line("DTSTART:" + it.StartsAt.UTC().Format("20060102T150405Z"))
		line("DTEND:" + it.EndsAt.UTC().Format("20060102T150405Z"))
		line("SUMMARY:" + icsText(it.Title))
		line("LOCATION:" + icsText(it.Room))
		desc := it.Description
		if len(it.People) > 0 {
			desc = strings.TrimSpace("With " + strings.Join(it.People, ", ") + "\n\n" + desc)
		}
		if desc != "" {
			line("DESCRIPTION:" + icsText(desc))
		}
		if it.EventType != "" {
			line("CATEGORIES:" + icsText(it.EventType))
		}
		if it.Status == "cancelled" {
			line("STATUS:CANCELLED")
		} else {
			line("STATUS:CONFIRMED")
		}
		line("SEQUENCE:" + fmt.Sprint(p.Version))
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String()
}

func icsText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}
