// Package events holds the domain-oriented repository and service seams for
// Rally's events functions (organizations, conventions, editions,
// memberships, programme — once the first vertical slice lands).
//
// In the scaffold this package is intentionally thin: it exports an
// OrganizationService that the HTTP layer calls for the Dashboard /
// Organizations placeholder pages. Larger domain modules (submissions,
// scheduling, publications) will arrive as separate files inside this
// package rather than a tangle of new top-level packages.
//
// The name "events" is deliberate — the repository folder is "Rally" but
// internal code uses the neutral ecosystem vocabulary.
package events

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Organization is the API-shaped projection of an organizations row.
type Organization struct {
	ID          uuid.UUID `json:"id"`
	Slug        string    `json:"slug"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role,omitempty"`
}

// OrganizationService lists organizations visible to the caller. The
// scaffold only supports the "list my organizations" read; create / update
// land with the organization-management vertical slice.
type OrganizationService struct {
	pool *pgxpool.Pool
}

// NewOrganizationService returns a service bound to the given pool.
func NewOrganizationService(pool *pgxpool.Pool) *OrganizationService {
	return &OrganizationService{pool: pool}
}

// ListVisible returns the organizations the authenticated user has a
// membership in, with the highest-privilege role per org. Membership
// filtering happens in SQL so the service never has to trust a client-sent
// organization id.
func (s *OrganizationService) ListVisible(ctx context.Context) ([]Organization, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT o.id,
		       o.slug,
		       o.display_name,
		       COALESCE(
		           -- Deterministic role precedence; refine when the full
		           -- role catalog settles.
		           MIN(ra.role) FILTER (
		               WHERE ra.role = 'organization_admin'
		           ),
		           MIN(ra.role)
		       ) AS role
		  FROM organizations o
		  JOIN memberships   m  ON m.organization_id = o.id
		                       AND m.user_id = $1
		                       AND m.status  = 'active'
		  LEFT JOIN role_assignments ra
		         ON ra.organization_id = o.id
		        AND ra.user_id         = $1
		        AND ra.edition_id IS NULL
		 GROUP BY o.id, o.slug, o.display_name
		 ORDER BY o.display_name ASC
	`, user.ID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()

	out := make([]Organization, 0, 4)
	for rows.Next() {
		var o Organization
		var role *string
		if err := rows.Scan(&o.ID, &o.Slug, &o.DisplayName, &role); err != nil {
			return nil, httperror.Internal(err)
		}
		if role != nil {
			o.Role = *role
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	return out, nil
}

// Get returns a single organization the caller can see, or 404 otherwise.
// Used by handlers that need to resolve the authoritative organization_id
// for a slug before applying permission checks.
func (s *OrganizationService) Get(ctx context.Context, id uuid.UUID) (*Organization, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	var o Organization
	err = s.pool.QueryRow(ctx, `
		SELECT o.id, o.slug, o.display_name
		  FROM organizations o
		  JOIN memberships m
		    ON m.organization_id = o.id
		   AND m.user_id = $2
		   AND m.status = 'active'
		 WHERE o.id = $1
	`, id, user.ID).Scan(&o.ID, &o.Slug, &o.DisplayName)
	switch {
	case err == nil:
		return &o, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("organization")
	default:
		return nil, httperror.Internal(err)
	}
}
