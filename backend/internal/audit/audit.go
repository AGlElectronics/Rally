// Package audit writes append-only audit_events rows inside the same
// transaction as the state change they describe. Callers pass the pgx.Tx
// (or the pool) that owns the surrounding write, so an audit row commits
// or rolls back atomically with the mutation.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AGlElectronics/rally/backend/internal/auth"
)

// Querier is the minimal pgx-ish surface Record needs. Both *pgxpool.Pool
// and pgx.Tx satisfy it, so audit calls can live inside or outside a
// surrounding transaction without a wrapper.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Record inserts a single audit_events row. `before`/`after` may be any
// JSON-serializable value or nil; both are stored as JSONB. The actor is
// read from the request context (see auth.ActorSnapshotFromContext).
//
// resourceType is a free string here but call sites MUST use the typed
// constants in internal/domain.ResourceType to prevent vocabulary drift.
func Record(
	ctx context.Context,
	q Querier,
	organizationID uuid.UUID,
	editionID *uuid.UUID,
	action string,
	resourceType string,
	resourceID uuid.UUID,
	before, after any,
	note string,
) error {
	beforeJSON, err := marshal(before)
	if err != nil {
		return fmt.Errorf("marshal audit before: %w", err)
	}
	afterJSON, err := marshal(after)
	if err != nil {
		return fmt.Errorf("marshal audit after: %w", err)
	}

	snap := auth.ActorSnapshotFromContext(ctx)
	var actorIssuer, actorSubject, actorName *string
	if snap.Issuer != "" {
		actorIssuer = &snap.Issuer
	}
	if snap.Subject != "" {
		actorSubject = &snap.Subject
	}
	if snap.DisplayName != "" {
		actorName = &snap.DisplayName
	}

	_, err = q.Exec(ctx, `
		INSERT INTO audit_events (
			organization_id, edition_id,
			actor_user_id, actor_issuer, actor_subject, actor_display_name,
			action, resource_type, resource_id,
			before_json, after_json, note
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		organizationID, editionID,
		snap.UserID, actorIssuer, actorSubject, actorName,
		action, resourceType, resourceID,
		beforeJSON, afterJSON, note,
	)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
