package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
	"github.com/AGlElectronics/rally/backend/internal/telegram"
)

// ScheduleSettings sizes the grid and sets what each rule does.
type ScheduleSettings struct {
	DayStartMinute int                 `json:"day_start_minute"`
	DayEndMinute   int                 `json:"day_end_minute"`
	SnapMinutes    int                 `json:"snap_minutes"`
	Rules          map[string]RuleMode `json:"rules"`
}

// SchedulePerson is someone presenting an item.
type SchedulePerson struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
}

// ScheduleItem is one block on the draft grid.
type ScheduleItem struct {
	ID           uuid.UUID        `json:"id"`
	SubmissionID *uuid.UUID       `json:"submission_id"`
	RoomID       uuid.UUID        `json:"room_id"`
	EventTypeID  *uuid.UUID       `json:"event_type_id"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	StartsAt     time.Time        `json:"starts_at"`
	EndsAt       time.Time        `json:"ends_at"`
	StaffOnly    bool             `json:"staff_only"`
	Status       string           `json:"status"`
	People       []SchedulePerson `json:"people"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// UnscheduledSubmission is an accepted submission not yet on the grid.
type UnscheduledSubmission struct {
	ID              uuid.UUID        `json:"id"`
	Title           string           `json:"title"`
	CfPTitle        string           `json:"cfp_title"`
	EventTypeID     *uuid.UUID       `json:"event_type_id"`
	DurationMinutes int              `json:"duration_minutes"`
	People          []SchedulePerson `json:"people"`
}

// PublishedInfo describes the version the public currently sees.
type PublishedInfo struct {
	Version     int       `json:"version"`
	Kind        string    `json:"kind"`
	Note        string    `json:"note"`
	PublishedAt time.Time `json:"published_at"`
}

// Schedule is everything the builder needs in one response.
type Schedule struct {
	Edition      Edition                 `json:"edition"`
	Settings     ScheduleSettings        `json:"settings"`
	Rooms        []Room                  `json:"rooms"`
	EventTypes   []EventType             `json:"event_types"`
	Items        []ScheduleItem          `json:"items"`
	Unscheduled  []UnscheduledSubmission `json:"unscheduled"`
	Conflicts    []Conflict              `json:"conflicts"`
	Published    *PublishedInfo          `json:"published"`
	DraftChanged bool                    `json:"draft_changed"`
	// PublicEnabled says whether the public schedule API is switched on.
	PublicEnabled bool `json:"public_enabled"`
	// TelegramConfigured is false when sends are dry runs.
	TelegramConfigured bool `json:"telegram_configured"`
}

// ScheduleService builds, publishes and runs the schedule.
type ScheduleService struct {
	pool *pgxpool.Pool
	tg   *telegram.Client
	// publicEnabled reports whether the public schedule API is on.
	publicEnabled func() bool
	now           func() time.Time
}

// NewScheduleService returns a service bound to pool. tg may be nil (dry
// runs); publicEnabled may be nil (off).
func NewScheduleService(pool *pgxpool.Pool, tg *telegram.Client, publicEnabled func() bool) *ScheduleService {
	if publicEnabled == nil {
		publicEnabled = func() bool { return false }
	}
	return &ScheduleService{pool: pool, tg: tg, publicEnabled: publicEnabled, now: time.Now}
}

// PublicEnabled reports whether the public schedule API is on.
func (s *ScheduleService) PublicEnabled() bool { return s.publicEnabled() }

func getEdition(ctx context.Context, q querier, id uuid.UUID) (*Edition, error) {
	var e Edition
	err := q.QueryRow(ctx, `SELECT id, organization_id, convention_id, slug, title, timezone, status, start_at, end_at
		FROM editions WHERE id=$1`, id).Scan(&e.ID, &e.OrganizationID, &e.ConventionID, &e.Slug, &e.Title,
		&e.Timezone, &e.Status, &e.StartAt, &e.EndAt)
	switch {
	case err == nil:
		return &e, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("edition")
	default:
		return nil, httperror.Internal(err)
	}
}

func editionLocation(e *Edition) *time.Location {
	if loc, err := time.LoadLocation(e.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// lockSchedule creates the edition's schedule row on first use and locks it,
// so concurrent drops are checked one after another.
func lockSchedule(ctx context.Context, tx pgx.Tx, orgID, editionID uuid.UUID) (ScheduleSettings, *time.Time, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO schedules (edition_id, organization_id) VALUES ($1,$2)
		ON CONFLICT (edition_id) DO NOTHING`, editionID, orgID); err != nil {
		return ScheduleSettings{}, nil, httperror.Internal(err)
	}
	return readSettings(ctx, tx, editionID, true)
}

func readSettings(ctx context.Context, q querier, editionID uuid.UUID, forUpdate bool) (ScheduleSettings, *time.Time, error) {
	st := ScheduleSettings{DayStartMinute: 540, DayEndMinute: 1380, SnapMinutes: 15, Rules: DefaultRules()}
	sql := `SELECT day_start_minute, day_end_minute, snap_minutes, rules, draft_updated_at FROM schedules WHERE edition_id=$1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	var raw []byte
	var draftAt *time.Time
	err := q.QueryRow(ctx, sql, editionID).Scan(&st.DayStartMinute, &st.DayEndMinute, &st.SnapMinutes, &raw, &draftAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil, nil
	}
	if err != nil {
		return st, nil, httperror.Internal(err)
	}
	stored := map[string]RuleMode{}
	_ = json.Unmarshal(raw, &stored)
	for k, v := range stored {
		if _, known := st.Rules[k]; known {
			st.Rules[k] = v
		}
	}
	return st, draftAt, nil
}

const itemColumns = `id, submission_id, room_id, event_type_id, title, description, starts_at, ends_at,
	staff_only, status, updated_at`

func scanItem(row pgx.Row) (*ScheduleItem, error) {
	var it ScheduleItem
	err := row.Scan(&it.ID, &it.SubmissionID, &it.RoomID, &it.EventTypeID, &it.Title, &it.Description,
		&it.StartsAt, &it.EndsAt, &it.StaffOnly, &it.Status, &it.UpdatedAt)
	switch {
	case err == nil:
		it.People = []SchedulePerson{}
		return &it, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("schedule item")
	default:
		return nil, httperror.Internal(err)
	}
}

// peopleOf returns the accepted people of each submission, owner first.
func peopleOf(ctx context.Context, q querier, submissionIDs []uuid.UUID) (map[uuid.UUID][]SchedulePerson, error) {
	out := map[uuid.UUID][]SchedulePerson{}
	if len(submissionIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT sp.submission_id, u.id, u.display_name
		FROM submission_people sp JOIN users u ON u.id = sp.user_id
		WHERE sp.status = 'accepted' AND sp.submission_id = ANY($1)
		ORDER BY (sp.role = 'owner') DESC, sp.created_at`, submissionIDs)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var p SchedulePerson
		if err := rows.Scan(&sid, &p.UserID, &p.Name); err != nil {
			return nil, httperror.Internal(err)
		}
		out[sid] = append(out[sid], p)
	}
	return out, rows.Err()
}

func listItems(ctx context.Context, q querier, editionID uuid.UUID) ([]ScheduleItem, error) {
	rows, err := q.Query(ctx, `SELECT `+itemColumns+` FROM schedule_items WHERE edition_id=$1 ORDER BY starts_at, title`, editionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	items := []ScheduleItem{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, *it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	var subIDs []uuid.UUID
	for _, it := range items {
		if it.SubmissionID != nil {
			subIDs = append(subIDs, *it.SubmissionID)
		}
	}
	people, err := peopleOf(ctx, q, subIDs)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].SubmissionID != nil {
			if ps := people[*items[i].SubmissionID]; ps != nil {
				items[i].People = ps
			}
		}
	}
	return items, nil
}

func listUnscheduled(ctx context.Context, q querier, editionID uuid.UUID) ([]UnscheduledSubmission, error) {
	rows, err := q.Query(ctx, `SELECT s.id, s.title, c.title, s.event_type_id,
			COALESCE(s.duration_minutes, et.default_minutes, 60)
		FROM submissions s
		JOIN call_for_papers c ON c.id = s.cfp_id
		LEFT JOIN event_types et ON et.id = s.event_type_id
		WHERE s.edition_id = $1 AND s.state = 'accepted'
		  AND NOT EXISTS (SELECT 1 FROM schedule_items i WHERE i.submission_id = s.id)
		ORDER BY s.decided_at NULLS LAST, s.title`, editionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	out := []UnscheduledSubmission{}
	for rows.Next() {
		var u UnscheduledSubmission
		if err := rows.Scan(&u.ID, &u.Title, &u.CfPTitle, &u.EventTypeID, &u.DurationMinutes); err != nil {
			rows.Close()
			return nil, httperror.Internal(err)
		}
		u.People = []SchedulePerson{}
		out = append(out, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	ids := make([]uuid.UUID, len(out))
	for i := range out {
		ids[i] = out[i].ID
	}
	people, err := peopleOf(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if ps := people[out[i].ID]; ps != nil {
			out[i].People = ps
		}
	}
	return out, nil
}

func latestVersion(ctx context.Context, q querier, editionID uuid.UUID) (*PublishedInfo, error) {
	var p PublishedInfo
	err := q.QueryRow(ctx, `SELECT version, kind, note, published_at FROM schedule_versions
		WHERE edition_id=$1 ORDER BY version DESC LIMIT 1`, editionID).Scan(&p.Version, &p.Kind, &p.Note, &p.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, httperror.Internal(err)
	}
	return &p, nil
}

// ruleWorldFor turns rooms, types and settings into the checker's view.
func ruleWorldFor(st ScheduleSettings, rooms []Room, types []EventType) ruleWorld {
	w := ruleWorld{Rules: st.Rules, Types: map[uuid.UUID]ruleType{}, Rooms: map[uuid.UUID]string{}}
	for _, r := range rooms {
		w.Rooms[r.ID] = r.Name
	}
	for _, t := range types {
		w.Types[t.ID] = ruleType{Name: t.Name, Gap: time.Duration(t.GapAfterMinutes) * time.Minute, RoomIDs: t.RoomIDs}
	}
	return w
}

func toRuleItems(items []ScheduleItem) []ruleItem {
	out := make([]ruleItem, 0, len(items))
	for _, it := range items {
		ri := ruleItem{ID: it.ID, Title: it.Title, RoomID: it.RoomID, EventTypeID: it.EventTypeID,
			Start: it.StartsAt, End: it.EndsAt, Cancelled: it.Status == "cancelled"}
		for _, p := range it.People {
			ri.People = append(ri.People, rulePerson(p))
		}
		out = append(out, ri)
	}
	return out
}

// load assembles the builder view inside q (a tx or the pool).
func (s *ScheduleService) load(ctx context.Context, q querier, editionID uuid.UUID) (*Schedule, error) {
	ed, err := getEdition(ctx, q, editionID)
	if err != nil {
		return nil, err
	}
	st, draftAt, err := readSettings(ctx, q, editionID, false)
	if err != nil {
		return nil, err
	}
	rooms, err := listRooms(ctx, q, editionID)
	if err != nil {
		return nil, err
	}
	types, err := listEventTypes(ctx, q, editionID)
	if err != nil {
		return nil, err
	}
	items, err := listItems(ctx, q, editionID)
	if err != nil {
		return nil, err
	}
	unscheduled, err := listUnscheduled(ctx, q, editionID)
	if err != nil {
		return nil, err
	}
	pub, err := latestVersion(ctx, q, editionID)
	if err != nil {
		return nil, err
	}
	out := &Schedule{
		Edition: *ed, Settings: st, Rooms: rooms, EventTypes: types, Items: items, Unscheduled: unscheduled,
		Conflicts:          checkConflicts(toRuleItems(items), ruleWorldFor(st, rooms, types), nil),
		Published:          pub,
		PublicEnabled:      s.publicEnabled(),
		TelegramConfigured: s.tg.Configured(),
	}
	switch {
	case pub == nil:
		out.DraftChanged = len(items) > 0
	case draftAt != nil:
		out.DraftChanged = draftAt.After(pub.PublishedAt)
	}
	return out, nil
}

// Get returns the draft schedule of an edition for staff.
func (s *ScheduleService) Get(ctx context.Context, editionID uuid.UUID) (*Schedule, error) {
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	return s.load(ctx, s.pool, editionID)
}

// UpdateSettings changes grid size, snap and rule modes. Requires schedule.edit.
func (s *ScheduleService) UpdateSettings(ctx context.Context, editionID uuid.UUID, in ScheduleSettings) (*Schedule, error) {
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, orgID, editionID); err != nil {
		return nil, err
	}
	issues := map[string]string{}
	if in.DayStartMinute < 0 || in.DayStartMinute > 1439 {
		issues["day_start_minute"] = "must be within the day"
	}
	if in.DayEndMinute < 1 || in.DayEndMinute > 1440 {
		issues["day_end_minute"] = "must be within the day"
	} else if in.DayEndMinute <= in.DayStartMinute {
		issues["day_end_minute"] = "must be after the day start"
	}
	switch in.SnapMinutes {
	case 5, 10, 15, 30:
	default:
		issues["snap_minutes"] = "must be 5, 10, 15 or 30"
	}
	rules := DefaultRules()
	for k, v := range in.Rules {
		if _, known := rules[k]; !known {
			issues["rules."+k] = "is not a known rule"
			continue
		}
		if v != RuleBlock && v != RuleWarn && v != RuleOff {
			issues["rules."+k] = "must be block, warn or off"
			continue
		}
		rules[k] = v
	}
	if err := validationIfAny("invalid schedule settings", issues); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(rules)
	var out *Schedule
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, _, err := lockSchedule(ctx, tx, orgID, editionID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE schedules SET day_start_minute=$2, day_end_minute=$3, snap_minutes=$4,
			rules=$5, updated_at=now() WHERE edition_id=$1`, editionID, in.DayStartMinute, in.DayEndMinute, in.SnapMinutes, raw); err != nil {
			return httperror.Internal(err)
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "schedule.settings", string(domain.ResourceSchedule),
			editionID, nil, in, ""); err != nil {
			return err
		}
		out, err = s.load(ctx, tx, editionID)
		return err
	})
	return out, wrapTxErr(err)
}

// optionalUUID tells "absent" (keep) apart from null (clear) in a PATCH.
type optionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (o *optionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var id uuid.UUID
	if err := json.Unmarshal(b, &id); err != nil {
		return err
	}
	o.Value = &id
	return nil
}

// ItemInput creates (POST) or changes (PATCH) an item. On create either
// SubmissionID or Title is required, plus RoomID and StartsAt; the length
// comes from EndsAt, DurationMinutes, the event type or 60 minutes.
type ItemInput struct {
	SubmissionID    *uuid.UUID   `json:"submission_id"`
	Title           *string      `json:"title"`
	Description     *string      `json:"description"`
	EventTypeID     optionalUUID `json:"event_type_id"`
	RoomID          *uuid.UUID   `json:"room_id"`
	StartsAt        *time.Time   `json:"starts_at"`
	EndsAt          *time.Time   `json:"ends_at"`
	DurationMinutes *int         `json:"duration_minutes"`
	StaffOnly       *bool        `json:"staff_only"`
}

// blockedError refuses a change that breaks a blocking rule.
func blockedError(cs []Conflict) error {
	msg := "this breaks a schedule rule"
	for _, c := range cs {
		if c.Severity == RuleBlock {
			msg = c.Message
			break
		}
	}
	return httperror.Conflict(msg).WithDetails(map[string]any{"conflicts": cs})
}

// checkDraft re-checks rules for the changed items after a write inside tx.
func checkDraft(ctx context.Context, tx pgx.Tx, editionID uuid.UUID, st ScheduleSettings, focus map[uuid.UUID]bool) error {
	rooms, err := listRooms(ctx, tx, editionID)
	if err != nil {
		return err
	}
	types, err := listEventTypes(ctx, tx, editionID)
	if err != nil {
		return err
	}
	items, err := listItems(ctx, tx, editionID)
	if err != nil {
		return err
	}
	cs := checkConflicts(toRuleItems(items), ruleWorldFor(st, rooms, types), focus)
	if hasBlocking(cs) {
		return blockedError(cs)
	}
	return nil
}

func roomInEdition(ctx context.Context, q querier, roomID, editionID uuid.UUID) error {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM rooms WHERE id=$1 AND edition_id=$2)`, roomID, editionID).Scan(&ok); err != nil {
		return httperror.Internal(err)
	}
	if !ok {
		return httperror.Validation("invalid item", map[string]string{"room_id": "is not a room of this edition"})
	}
	return nil
}

func eventTypeInEdition(ctx context.Context, q querier, id *uuid.UUID, editionID uuid.UUID) (int, error) {
	if id == nil {
		return 0, nil
	}
	var minutes int
	err := q.QueryRow(ctx, `SELECT default_minutes FROM event_types WHERE id=$1 AND edition_id=$2`, *id, editionID).Scan(&minutes)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, httperror.Validation("invalid item", map[string]string{"event_type_id": "is not an event type of this edition"})
	}
	if err != nil {
		return 0, httperror.Internal(err)
	}
	return minutes, nil
}

func checkSpan(start, end time.Time) error {
	switch {
	case !start.Before(end):
		return httperror.Validation("invalid item", map[string]string{"ends_at": "must be after the start"})
	case end.Sub(start) > 24*time.Hour:
		return httperror.Validation("invalid item", map[string]string{"ends_at": "an item can last at most 24 hours"})
	}
	return nil
}

func touchDraft(ctx context.Context, tx pgx.Tx, editionID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE schedules SET draft_updated_at=now() WHERE edition_id=$1`, editionID); err != nil {
		return httperror.Internal(err)
	}
	return nil
}

// submissionBlurb picks a public description: the abstract, else the first
// long-text answer of the call's form.
func submissionBlurb(ctx context.Context, q querier, submissionID uuid.UUID) string {
	var abstract string
	var formRaw, answersRaw []byte
	if err := q.QueryRow(ctx, `SELECT s.abstract, c.form, s.answers FROM submissions s
		JOIN call_for_papers c ON c.id = s.cfp_id WHERE s.id=$1`, submissionID).Scan(&abstract, &formRaw, &answersRaw); err != nil {
		return ""
	}
	if strings.TrimSpace(abstract) != "" {
		return abstract
	}
	var form Form
	answers := map[string]any{}
	_ = json.Unmarshal(formRaw, &form)
	_ = json.Unmarshal(answersRaw, &answers)
	for _, f := range form.Fields {
		if f.Type == FieldLongText && !f.Archived {
			if v, ok := answers[f.ID].(string); ok && strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return ""
}

// CreateItem puts a submission or a staff item on the grid. Requires
// schedule.edit; refused when it breaks a blocking rule.
func (s *ScheduleService) CreateItem(ctx context.Context, editionID uuid.UUID, in ItemInput) (*Schedule, error) {
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
	issues := map[string]string{}
	if in.RoomID == nil {
		issues["room_id"] = "is required"
	}
	if in.StartsAt == nil {
		issues["starts_at"] = "is required"
	}
	if in.SubmissionID == nil && (in.Title == nil || strings.TrimSpace(*in.Title) == "") {
		issues["title"] = "is required"
	}
	if err := validationIfAny("invalid item", issues); err != nil {
		return nil, err
	}

	var out *Schedule
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		st, _, err := lockSchedule(ctx, tx, orgID, editionID)
		if err != nil {
			return err
		}
		if err := roomInEdition(ctx, tx, *in.RoomID, editionID); err != nil {
			return err
		}
		title, description := "", ""
		typeID := in.EventTypeID.Value
		minutes := 0
		if in.SubmissionID != nil {
			var subEdition uuid.UUID
			var state string
			var subType *uuid.UUID
			var subMinutes *int
			err := tx.QueryRow(ctx, `SELECT edition_id, state, title, event_type_id, duration_minutes
				FROM submissions WHERE id=$1`, *in.SubmissionID).Scan(&subEdition, &state, &title, &subType, &subMinutes)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && subEdition != editionID) {
				return httperror.NotFound("submission")
			}
			if err != nil {
				return httperror.Internal(err)
			}
			if state != "accepted" {
				return httperror.Conflict("only accepted submissions can be scheduled")
			}
			var taken bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schedule_items WHERE submission_id=$1)`, *in.SubmissionID).Scan(&taken); err != nil {
				return httperror.Internal(err)
			}
			if taken {
				return httperror.Conflict("this submission is already on the schedule")
			}
			if !in.EventTypeID.Set {
				typeID = subType
			}
			if subMinutes != nil {
				minutes = *subMinutes
			}
			description = submissionBlurb(ctx, tx, *in.SubmissionID)
		}
		if in.Title != nil && strings.TrimSpace(*in.Title) != "" {
			title = strings.TrimSpace(*in.Title)
		}
		if in.Description != nil {
			description = strings.TrimSpace(*in.Description)
		}
		typeMinutes, err := eventTypeInEdition(ctx, tx, typeID, editionID)
		if err != nil {
			return err
		}
		start := *in.StartsAt
		var end time.Time
		switch {
		case in.EndsAt != nil:
			end = *in.EndsAt
		case in.DurationMinutes != nil:
			end = start.Add(time.Duration(*in.DurationMinutes) * time.Minute)
		case minutes > 0:
			end = start.Add(time.Duration(minutes) * time.Minute)
		case typeMinutes > 0:
			end = start.Add(time.Duration(typeMinutes) * time.Minute)
		default:
			end = start.Add(time.Hour)
		}
		if err := checkSpan(start, end); err != nil {
			return err
		}
		staffOnly := in.StaffOnly != nil && *in.StaffOnly
		it, err := scanItem(tx.QueryRow(ctx, `INSERT INTO schedule_items
			(organization_id, edition_id, submission_id, room_id, event_type_id, title, description,
			 starts_at, ends_at, staff_only, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+itemColumns,
			orgID, editionID, in.SubmissionID, *in.RoomID, typeID, title, description, start, end, staffOnly, u.ID))
		if err != nil {
			return err
		}
		if err := checkDraft(ctx, tx, editionID, st, map[uuid.UUID]bool{it.ID: true}); err != nil {
			return err
		}
		if err := touchDraft(ctx, tx, editionID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "schedule_item.create",
			string(domain.ResourceScheduleItem), it.ID, nil, it, ""); err != nil {
			return err
		}
		out, err = s.load(ctx, tx, editionID)
		return err
	})
	return out, wrapTxErr(err)
}

func itemOwner(ctx context.Context, tx pgx.Tx, id uuid.UUID) (orgID, editionID uuid.UUID, err error) {
	err = tx.QueryRow(ctx, `SELECT organization_id, edition_id FROM schedule_items WHERE id=$1`, id).Scan(&orgID, &editionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, httperror.NotFound("schedule item")
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, httperror.Internal(err)
	}
	return orgID, editionID, nil
}

// UpdateItem moves, resizes or edits an item. Requires schedule.edit.
func (s *ScheduleService) UpdateItem(ctx context.Context, id uuid.UUID, in ItemInput) (*Schedule, error) {
	if in.SubmissionID != nil {
		return nil, httperror.Validation("invalid item", map[string]string{"submission_id": "can't be changed"})
	}
	var out *Schedule
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		orgID, editionID, err := itemOwner(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, orgID, editionID); err != nil {
			return err
		}
		st, _, err := lockSchedule(ctx, tx, orgID, editionID)
		if err != nil {
			return err
		}
		before, err := scanItem(tx.QueryRow(ctx, `SELECT `+itemColumns+` FROM schedule_items WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		after := *before
		if in.RoomID != nil {
			if err := roomInEdition(ctx, tx, *in.RoomID, editionID); err != nil {
				return err
			}
			after.RoomID = *in.RoomID
		}
		if in.EventTypeID.Set {
			if _, err := eventTypeInEdition(ctx, tx, in.EventTypeID.Value, editionID); err != nil {
				return err
			}
			after.EventTypeID = in.EventTypeID.Value
		}
		if in.Title != nil {
			t := strings.TrimSpace(*in.Title)
			if t == "" {
				return httperror.Validation("invalid item", map[string]string{"title": "is required"})
			}
			after.Title = t
		}
		if in.Description != nil {
			after.Description = strings.TrimSpace(*in.Description)
		}
		if in.StaffOnly != nil {
			after.StaffOnly = *in.StaffOnly
		}
		length := before.EndsAt.Sub(before.StartsAt)
		if in.StartsAt != nil {
			after.StartsAt = *in.StartsAt
			after.EndsAt = after.StartsAt.Add(length)
		}
		switch {
		case in.EndsAt != nil:
			after.EndsAt = *in.EndsAt
		case in.DurationMinutes != nil:
			after.EndsAt = after.StartsAt.Add(time.Duration(*in.DurationMinutes) * time.Minute)
		}
		if err := checkSpan(after.StartsAt, after.EndsAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE schedule_items SET room_id=$2, event_type_id=$3, title=$4, description=$5,
			starts_at=$6, ends_at=$7, staff_only=$8, updated_at=now() WHERE id=$1`,
			id, after.RoomID, after.EventTypeID, after.Title, after.Description, after.StartsAt, after.EndsAt, after.StaffOnly); err != nil {
			return httperror.Internal(err)
		}
		if err := checkDraft(ctx, tx, editionID, st, map[uuid.UUID]bool{id: true}); err != nil {
			return err
		}
		if err := touchDraft(ctx, tx, editionID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "schedule_item.update",
			string(domain.ResourceScheduleItem), id, before, after, ""); err != nil {
			return err
		}
		out, err = s.load(ctx, tx, editionID)
		return err
	})
	return out, wrapTxErr(err)
}

// DeleteItem takes an item off the grid; a submission goes back to the
// unscheduled list. Requires schedule.edit.
func (s *ScheduleService) DeleteItem(ctx context.Context, id uuid.UUID) (*Schedule, error) {
	var out *Schedule
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		orgID, editionID, err := itemOwner(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, orgID, editionID); err != nil {
			return err
		}
		if _, _, err := lockSchedule(ctx, tx, orgID, editionID); err != nil {
			return err
		}
		before, err := scanItem(tx.QueryRow(ctx, `SELECT `+itemColumns+` FROM schedule_items WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE announcements SET item_id=NULL WHERE item_id=$1`, id); err != nil {
			return httperror.Internal(err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schedule_items WHERE id=$1`, id); err != nil {
			return httperror.Internal(err)
		}
		if err := touchDraft(ctx, tx, editionID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "schedule_item.delete",
			string(domain.ResourceScheduleItem), id, before, nil, ""); err != nil {
			return err
		}
		out, err = s.load(ctx, tx, editionID)
		return err
	})
	return out, wrapTxErr(err)
}
