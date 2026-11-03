package events

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Edition is the API-shaped projection of an editions row (read-only here).
type Edition struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ConventionID   uuid.UUID  `json:"convention_id"`
	Slug           string     `json:"slug"`
	Title          string     `json:"title"`
	Timezone       string     `json:"timezone"`
	Status         string     `json:"status"`
	StartAt        *time.Time `json:"start_at"`
	EndAt          *time.Time `json:"end_at"`
}

// EditionService exposes read access to editions. Create/update arrive with
// the edition-management slice.
type EditionService struct {
	pool *pgxpool.Pool
}

// NewEditionService returns a service bound to pool.
func NewEditionService(pool *pgxpool.Pool) *EditionService {
	return &EditionService{pool: pool}
}

// ListVisible returns editions the caller can see: every edition of an
// organization they are an active member of, plus any edition (in any
// organization) that currently has an open call for papers, so prospective
// speakers can discover where to submit.
func (s *EditionService) ListVisible(ctx context.Context) ([]Edition, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.organization_id, e.convention_id, e.slug, e.title,
		       e.timezone, e.status, e.start_at, e.end_at
		  FROM editions e
		 WHERE EXISTS (
		           SELECT 1 FROM memberships m
		            WHERE m.organization_id = e.organization_id
		              AND m.user_id = $1
		              AND m.status  = 'active')
		    OR EXISTS (
		           SELECT 1 FROM call_for_papers c
		            WHERE c.edition_id = e.id
		              AND c.status = 'open')
		 ORDER BY e.start_at NULLS LAST, e.title ASC
	`, user.ID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()

	out := make([]Edition, 0, 4)
	for rows.Next() {
		var e Edition
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.ConventionID, &e.Slug, &e.Title,
			&e.Timezone, &e.Status, &e.StartAt, &e.EndAt); err != nil {
			return nil, httperror.Internal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	return out, nil
}

// querier is satisfied by *pgxpool.Pool and pgx.Tx.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// editionOwner resolves the owning organization of an edition from the
// database (never from client input). Returns a 404 if it doesn't exist.
func editionOwner(ctx context.Context, q querier, editionID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := q.QueryRow(ctx, `SELECT organization_id FROM editions WHERE id = $1`, editionID).Scan(&orgID)
	switch {
	case err == nil:
		return orgID, nil
	case errors.Is(err, pgx.ErrNoRows):
		return uuid.Nil, httperror.NotFound("edition")
	default:
		return uuid.Nil, httperror.Internal(err)
	}
}
