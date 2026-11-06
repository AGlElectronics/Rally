package events

import (
	"context"
	"errors"
	"strings"
	"time"
	_ "time/tzdata" // edition timezones must resolve in minimal containers

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

// Room is a room or stage of an edition.
type Room struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	EditionID      uuid.UUID `json:"edition_id"`
	Name           string    `json:"name"`
	Capacity       *int      `json:"capacity"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RoomInput is the create/replace payload.
type RoomInput struct {
	Name      string `json:"name"`
	Capacity  *int   `json:"capacity"`
	SortOrder int    `json:"sort_order"`
}

func (in *RoomInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	issues := map[string]string{}
	if in.Name == "" {
		issues["name"] = "is required"
	} else if len([]rune(in.Name)) > 80 {
		issues["name"] = "must be at most 80 characters"
	}
	if in.Capacity != nil && (*in.Capacity < 1 || *in.Capacity > 1000000) {
		issues["capacity"] = "must be a positive number"
	}
	return validationIfAny("invalid room", issues)
}

// RoomService manages rooms and the edition's dates.
type RoomService struct {
	pool *pgxpool.Pool
}

// NewRoomService returns a service bound to pool.
func NewRoomService(pool *pgxpool.Pool) *RoomService {
	return &RoomService{pool: pool}
}

const roomColumns = `id, organization_id, edition_id, name, capacity, sort_order, created_at, updated_at`

func scanRoom(row pgx.Row) (*Room, error) {
	var r Room
	err := row.Scan(&r.ID, &r.OrganizationID, &r.EditionID, &r.Name, &r.Capacity, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt)
	switch {
	case err == nil:
		return &r, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("room")
	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, httperror.Validation("invalid room", map[string]string{"name": "already exists in this edition"})
		}
		return nil, httperror.Internal(err)
	}
}

func listRooms(ctx context.Context, q querier, editionID uuid.UUID) ([]Room, error) {
	rows, err := q.Query(ctx, `SELECT `+roomColumns+` FROM rooms WHERE edition_id=$1 ORDER BY sort_order, name`, editionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []Room{}
	for rows.Next() {
		r, err := scanRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	return out, nil
}

// isScheduleStaff reports whether the caller may see the draft schedule.
func isScheduleStaff(u *auth.User, orgID, editionID uuid.UUID) bool {
	return u.HasForEdition(domain.PermScheduleEdit, orgID, editionID) ||
		u.HasForEdition(domain.PermSchedulePublish, orgID, editionID) ||
		isStaffForEdition(u, orgID, editionID)
}

// requireScheduleStaff resolves the edition owner and checks read access.
func requireScheduleStaff(ctx context.Context, q querier, editionID uuid.UUID) (uuid.UUID, *auth.User, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return uuid.Nil, nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, q, editionID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if !isScheduleStaff(u, orgID, editionID) {
		return uuid.Nil, nil, httperror.NotFound("edition")
	}
	return orgID, u, nil
}

// List returns an edition's rooms.
func (s *RoomService) List(ctx context.Context, editionID uuid.UUID) ([]Room, error) {
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	return listRooms(ctx, s.pool, editionID)
}

// Create adds a room. Requires schedule.edit.
func (s *RoomService) Create(ctx context.Context, editionID uuid.UUID, in RoomInput) (*Room, error) {
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, orgID, editionID); err != nil {
		return nil, err
	}
	if err := in.normalize(); err != nil {
		return nil, err
	}
	var out *Room
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := scanRoom(tx.QueryRow(ctx, `INSERT INTO rooms (organization_id, edition_id, name, capacity, sort_order)
			VALUES ($1,$2,$3,$4,$5) RETURNING `+roomColumns, orgID, editionID, in.Name, in.Capacity, in.SortOrder))
		if err != nil {
			return err
		}
		out = r
		return audit.Record(ctx, tx, orgID, &editionID, "room.create", string(domain.ResourceRoom), r.ID, nil, r, "")
	})
	return out, wrapTxErr(err)
}

// Update replaces a room. Requires schedule.edit.
func (s *RoomService) Update(ctx context.Context, id uuid.UUID, in RoomInput) (*Room, error) {
	if err := in.normalize(); err != nil {
		return nil, err
	}
	var out *Room
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := scanRoom(tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, before.OrganizationID, before.EditionID); err != nil {
			return err
		}
		after, err := scanRoom(tx.QueryRow(ctx, `UPDATE rooms SET name=$2, capacity=$3, sort_order=$4, updated_at=now()
			WHERE id=$1 RETURNING `+roomColumns, id, in.Name, in.Capacity, in.SortOrder))
		if err != nil {
			return err
		}
		out = after
		return audit.Record(ctx, tx, after.OrganizationID, &after.EditionID, "room.update",
			string(domain.ResourceRoom), after.ID, before, after, "")
	})
	return out, wrapTxErr(err)
}

// Delete removes a room that has nothing scheduled in it.
func (s *RoomService) Delete(ctx context.Context, id uuid.UUID) error {
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := scanRoom(tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, before.OrganizationID, before.EditionID); err != nil {
			return err
		}
		var used int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM schedule_items WHERE room_id=$1`, id).Scan(&used); err != nil {
			return httperror.Internal(err)
		}
		if used > 0 {
			return httperror.Conflict("items are scheduled in this room; move them first")
		}
		if _, err := tx.Exec(ctx, `UPDATE event_types SET room_ids = array_remove(room_ids, $1) WHERE $1 = ANY(room_ids)`, id); err != nil {
			return httperror.Internal(err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM rooms WHERE id=$1`, id); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, before.OrganizationID, &before.EditionID, "room.delete",
			string(domain.ResourceRoom), before.ID, before, nil, "")
	})
	return wrapTxErr(err)
}

// EditionDatesInput sets the days and timezone the schedule covers.
type EditionDatesInput struct {
	Timezone string     `json:"timezone"`
	StartAt  *time.Time `json:"start_at"`
	EndAt    *time.Time `json:"end_at"`
}

// UpdateEditionDates changes an edition's timezone and date range. Requires
// edition.edit or schedule.edit.
func (s *RoomService) UpdateEditionDates(ctx context.Context, editionID uuid.UUID, in EditionDatesInput) (*Edition, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if !u.HasForEdition(domain.PermEditionEdit, orgID, editionID) && !u.HasForEdition(domain.PermScheduleEdit, orgID, editionID) {
		return nil, httperror.New(httperror.CodeForbidden, "permission denied")
	}
	in.Timezone = strings.TrimSpace(in.Timezone)
	issues := map[string]string{}
	if in.Timezone == "" {
		issues["timezone"] = "is required"
	} else if _, err := time.LoadLocation(in.Timezone); err != nil {
		issues["timezone"] = "is not a known timezone (e.g. Europe/Vienna)"
	}
	if in.StartAt != nil && in.EndAt != nil {
		if !in.StartAt.Before(*in.EndAt) {
			issues["end_at"] = "must be after the start"
		} else if in.EndAt.Sub(*in.StartAt) > 31*24*time.Hour {
			issues["end_at"] = "an edition can last at most 31 days"
		}
	}
	if err := validationIfAny("invalid dates", issues); err != nil {
		return nil, err
	}
	var out Edition
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `UPDATE editions SET timezone=$2, start_at=$3, end_at=$4, updated_at=now() WHERE id=$1
			RETURNING id, organization_id, convention_id, slug, title, timezone, status, start_at, end_at`,
			editionID, in.Timezone, in.StartAt, in.EndAt).Scan(&out.ID, &out.OrganizationID, &out.ConventionID,
			&out.Slug, &out.Title, &out.Timezone, &out.Status, &out.StartAt, &out.EndAt); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, orgID, &editionID, "edition.dates", string(domain.ResourceEdition), editionID, nil, out, "")
	})
	return &out, wrapTxErr(err)
}
