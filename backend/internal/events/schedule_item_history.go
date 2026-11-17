package events

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Per-schedule-item revision actions. Separate from the edition-wide
// published versions in schedule_publish.go; one row per change to one
// draft item.
const (
	RevisionCreate  = "create"
	RevisionUpdate  = "update"
	RevisionDelete  = "delete"
	RevisionLive    = "live"
	RevisionRestore = "restore"
)

// Revision is one entry in a schedule item's history.
type Revision struct {
	ID             uuid.UUID       `json:"id"`
	ScheduleItemID uuid.UUID       `json:"schedule_item_id"`
	EditionID      uuid.UUID       `json:"edition_id"`
	Action         string          `json:"action"`
	Note           string          `json:"note"`
	Actor          string          `json:"actor"`
	Before         json.RawMessage `json:"before,omitempty"`
	After          json.RawMessage `json:"after,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// RevisionSummary is a Revision without the before/after payload.
type RevisionSummary struct {
	ID             uuid.UUID `json:"id"`
	ScheduleItemID uuid.UUID `json:"schedule_item_id"`
	EditionID      uuid.UUID `json:"edition_id"`
	Action         string    `json:"action"`
	Note           string    `json:"note"`
	Actor          string    `json:"actor"`
	CreatedAt      time.Time `json:"created_at"`
}

// recordRevision writes one row to schedule_item_revisions inside tx. The
// actor is read from ctx, same as audit.Record. before/after may be nil.
func recordRevision(ctx context.Context, tx pgx.Tx, orgID, editionID, itemID uuid.UUID,
	action, note string, before, after any) error {
	beforeJSON, err := marshalJSONOrNil(before)
	if err != nil {
		return httperror.Internal(err)
	}
	afterJSON, err := marshalJSONOrNil(after)
	if err != nil {
		return httperror.Internal(err)
	}
	snap := auth.ActorSnapshotFromContext(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO schedule_item_revisions
		(organization_id, edition_id, schedule_item_id, action, note, before_data, after_data, actor_user_id, actor_display)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		orgID, editionID, itemID, action, note, beforeJSON, afterJSON, snap.UserID, snap.DisplayName)
	if err != nil {
		return httperror.Internal(err)
	}
	return nil
}

func marshalJSONOrNil(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// ListItemRevisions returns the history of one schedule item, newest first.
// Programme staff of the edition may read it; the item may have been
// deleted (we look the edition up in the revisions table too).
func (s *ScheduleService) ListItemRevisions(ctx context.Context, itemID uuid.UUID) ([]RevisionSummary, error) {
	editionID, orgID, err := s.revisionScope(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	_ = orgID
	rows, err := s.pool.Query(ctx, `SELECT id, schedule_item_id, edition_id, action, note,
			actor_display, created_at
		FROM schedule_item_revisions
		WHERE schedule_item_id = $1 ORDER BY created_at DESC, id DESC`, itemID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []RevisionSummary{}
	for rows.Next() {
		var r RevisionSummary
		if err := rows.Scan(&r.ID, &r.ScheduleItemID, &r.EditionID, &r.Action, &r.Note,
			&r.Actor, &r.CreatedAt); err != nil {
			return nil, httperror.Internal(err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetItemRevision returns one revision with its before/after data.
func (s *ScheduleService) GetItemRevision(ctx context.Context, itemID, revisionID uuid.UUID) (*Revision, error) {
	editionID, _, err := s.revisionScope(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if _, _, err := requireScheduleStaff(ctx, s.pool, editionID); err != nil {
		return nil, err
	}
	var r Revision
	err = s.pool.QueryRow(ctx, `SELECT id, schedule_item_id, edition_id, action, note,
			actor_display, before_data, after_data, created_at
		FROM schedule_item_revisions WHERE id = $1 AND schedule_item_id = $2`,
		revisionID, itemID).Scan(&r.ID, &r.ScheduleItemID, &r.EditionID, &r.Action, &r.Note,
		&r.Actor, &r.Before, &r.After, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httperror.NotFound("revision")
	}
	if err != nil {
		return nil, httperror.Internal(err)
	}
	return &r, nil
}

// revisionScope resolves the edition (and its owning org) an item lives in,
// falling back to the revisions table when the item itself is gone.
func (s *ScheduleService) revisionScope(ctx context.Context, itemID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var editionID, orgID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT edition_id, organization_id FROM schedule_items WHERE id=$1`,
		itemID).Scan(&editionID, &orgID)
	if err == nil {
		return editionID, orgID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, httperror.Internal(err)
	}
	err = s.pool.QueryRow(ctx, `SELECT edition_id, organization_id FROM schedule_item_revisions
		WHERE schedule_item_id=$1 ORDER BY created_at DESC LIMIT 1`, itemID).Scan(&editionID, &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, httperror.NotFound("schedule item")
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, httperror.Internal(err)
	}
	return editionID, orgID, nil
}
