package events

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// EventType is a kind of programme item (panel, stage show, workshop...).
// Its default length and changeover gap feed the schedule builder.
type EventType struct {
	ID              uuid.UUID `json:"id"`
	OrganizationID  uuid.UUID `json:"organization_id"`
	EditionID       uuid.UUID `json:"edition_id"`
	Name            string    `json:"name"`
	Color           string    `json:"color"`
	DefaultMinutes  int       `json:"default_minutes"`
	GapAfterMinutes int       `json:"gap_after_minutes"`
	SortOrder       int       `json:"sort_order"`
	// Rooms this type may use; empty = any room.
	RoomIDs   []uuid.UUID `json:"room_ids"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// EventTypeInput is the create/replace payload. A nil RoomIDs keeps the
// current rooms on update.
type EventTypeInput struct {
	Name            string       `json:"name"`
	Color           string       `json:"color"`
	DefaultMinutes  int          `json:"default_minutes"`
	GapAfterMinutes int          `json:"gap_after_minutes"`
	SortOrder       int          `json:"sort_order"`
	RoomIDs         *[]uuid.UUID `json:"room_ids"`
}

func (in *EventTypeInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Color = strings.TrimSpace(in.Color)
	if in.Color == "" {
		in.Color = "#6366f1"
	}
	if in.DefaultMinutes == 0 {
		in.DefaultMinutes = 60
	}
	issues := map[string]string{}
	if in.Name == "" {
		issues["name"] = "is required"
	} else if len([]rune(in.Name)) > 80 {
		issues["name"] = "must be at most 80 characters"
	}
	if !colorPattern.MatchString(in.Color) {
		issues["color"] = "must look like #aabbcc"
	}
	if in.DefaultMinutes < 5 || in.DefaultMinutes > 1440 {
		issues["default_minutes"] = "must be between 5 and 1440"
	}
	if in.GapAfterMinutes < 0 || in.GapAfterMinutes > 720 {
		issues["gap_after_minutes"] = "must be between 0 and 720"
	}
	return validationIfAny("invalid event type", issues)
}

// EventTypeService manages event types of an edition.
type EventTypeService struct {
	pool *pgxpool.Pool
}

// NewEventTypeService returns a service bound to pool.
func NewEventTypeService(pool *pgxpool.Pool) *EventTypeService {
	return &EventTypeService{pool: pool}
}

const eventTypeColumns = `id, organization_id, edition_id, name, color, default_minutes,
	gap_after_minutes, sort_order, room_ids, created_at, updated_at`

func scanEventType(row pgx.Row) (*EventType, error) {
	var t EventType
	err := row.Scan(&t.ID, &t.OrganizationID, &t.EditionID, &t.Name, &t.Color, &t.DefaultMinutes,
		&t.GapAfterMinutes, &t.SortOrder, &t.RoomIDs, &t.CreatedAt, &t.UpdatedAt)
	switch {
	case err == nil:
		if t.RoomIDs == nil {
			t.RoomIDs = []uuid.UUID{}
		}
		return &t, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("event type")
	default:
		return nil, mapUniqueName(err)
	}
}

func mapUniqueName(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return httperror.Validation("invalid event type", map[string]string{"name": "already exists in this edition"})
	}
	return httperror.Internal(err)
}

// listEventTypes returns an edition's event types in display order. Readable
// by anyone who can see a CfP of the edition, so no permission check here.
func listEventTypes(ctx context.Context, q querier, editionID uuid.UUID) ([]EventType, error) {
	rows, err := q.Query(ctx, `SELECT `+eventTypeColumns+` FROM event_types
		WHERE edition_id = $1 ORDER BY sort_order, name`, editionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []EventType{}
	for rows.Next() {
		t, err := scanEventType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	return out, nil
}

// List returns the event types of an edition (staff of the edition only).
func (s *EventTypeService) List(ctx context.Context, editionID uuid.UUID) ([]EventType, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if !isStaffForEdition(u, orgID, editionID) {
		return nil, httperror.NotFound("edition")
	}
	return listEventTypes(ctx, s.pool, editionID)
}

// Create adds an event type. Requires cfp.manage.
func (s *EventTypeService) Create(ctx context.Context, editionID uuid.UUID, in EventTypeInput) (*EventType, error) {
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermCfPManage, orgID, editionID); err != nil {
		return nil, err
	}
	if err := in.normalize(); err != nil {
		return nil, err
	}
	var out *EventType
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		rooms, err := checkRoomIDs(ctx, tx, editionID, in.RoomIDs, []uuid.UUID{})
		if err != nil {
			return err
		}
		t, err := scanEventType(tx.QueryRow(ctx, `INSERT INTO event_types
			(organization_id, edition_id, name, color, default_minutes, gap_after_minutes, sort_order, room_ids)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+eventTypeColumns,
			orgID, editionID, in.Name, in.Color, in.DefaultMinutes, in.GapAfterMinutes, in.SortOrder, rooms))
		if err != nil {
			return err
		}
		out = t
		return audit.Record(ctx, tx, orgID, &editionID, "event_type.create",
			string(domain.ResourceEventType), t.ID, nil, t, "")
	})
	return out, wrapTxErr(err)
}

// Update replaces an event type. Requires cfp.manage.
func (s *EventTypeService) Update(ctx context.Context, id uuid.UUID, in EventTypeInput) (*EventType, error) {
	if err := in.normalize(); err != nil {
		return nil, err
	}
	var out *EventType
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := scanEventType(tx.QueryRow(ctx, `SELECT `+eventTypeColumns+` FROM event_types WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermCfPManage, before.OrganizationID, before.EditionID); err != nil {
			return err
		}
		rooms, err := checkRoomIDs(ctx, tx, before.EditionID, in.RoomIDs, before.RoomIDs)
		if err != nil {
			return err
		}
		after, err := scanEventType(tx.QueryRow(ctx, `UPDATE event_types
			SET name=$2, color=$3, default_minutes=$4, gap_after_minutes=$5, sort_order=$6, room_ids=$7, updated_at=now()
			WHERE id=$1 RETURNING `+eventTypeColumns,
			id, in.Name, in.Color, in.DefaultMinutes, in.GapAfterMinutes, in.SortOrder, rooms))
		if err != nil {
			return err
		}
		out = after
		return audit.Record(ctx, tx, after.OrganizationID, &after.EditionID, "event_type.update",
			string(domain.ResourceEventType), after.ID, before, after, "")
	})
	return out, wrapTxErr(err)
}

// Delete removes an event type that no submission uses. Requires cfp.manage.
func (s *EventTypeService) Delete(ctx context.Context, id uuid.UUID) error {
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := scanEventType(tx.QueryRow(ctx, `SELECT `+eventTypeColumns+` FROM event_types WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermCfPManage, before.OrganizationID, before.EditionID); err != nil {
			return err
		}
		var used int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE event_type_id=$1`, id).Scan(&used); err != nil {
			return httperror.Internal(err)
		}
		if used > 0 {
			return httperror.Conflict("submissions use this event type; rename it instead")
		}
		if _, err := tx.Exec(ctx, `UPDATE call_for_papers SET event_type_ids = array_remove(event_type_ids, $1)
			WHERE $1 = ANY(event_type_ids)`, id); err != nil {
			return httperror.Internal(err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM event_types WHERE id=$1`, id); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, before.OrganizationID, &before.EditionID, "event_type.delete",
			string(domain.ResourceEventType), before.ID, before, nil, "")
	})
	return wrapTxErr(err)
}

// checkRoomIDs returns the room list to store: current when ids is nil,
// otherwise ids after checking each room belongs to the edition.
func checkRoomIDs(ctx context.Context, q querier, editionID uuid.UUID, ids *[]uuid.UUID, current []uuid.UUID) ([]uuid.UUID, error) {
	if ids == nil {
		if current == nil {
			return []uuid.UUID{}, nil
		}
		return current, nil
	}
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(*ids))
	for _, id := range *ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	var n int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM rooms WHERE edition_id=$1 AND id = ANY($2)`, editionID, out).Scan(&n); err != nil {
		return nil, httperror.Internal(err)
	}
	if n != len(out) {
		return nil, httperror.Validation("invalid event type", map[string]string{"room_ids": "contains a room of another edition"})
	}
	return out, nil
}

// allowedEventTypes returns the event types a CfP accepts (all of the
// edition's when the CfP doesn't restrict them).
func allowedEventTypes(ctx context.Context, q querier, c *CallForPapers) ([]EventType, error) {
	all, err := listEventTypes(ctx, q, c.EditionID)
	if err != nil || len(c.EventTypeIDs) == 0 {
		return all, err
	}
	keep := map[uuid.UUID]struct{}{}
	for _, id := range c.EventTypeIDs {
		keep[id] = struct{}{}
	}
	out := make([]EventType, 0, len(c.EventTypeIDs))
	for _, t := range all {
		if _, ok := keep[t.ID]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}
