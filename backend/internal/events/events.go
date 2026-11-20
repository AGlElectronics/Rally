// Package events holds the domain-oriented repository and service seams for
// Rally (organizations, conventions, editions, memberships, CfPs, schedule).
//
// The name "events" is deliberate — the repository folder is "Rally" but
// internal code uses the neutral ecosystem vocabulary.
package events

import (
	"context"
	"errors"
	"sort"

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

// OrganizationService lists organizations visible to the caller.
type OrganizationService struct {
	pool *pgxpool.Pool
}

// NewOrganizationService returns a service bound to the given pool.
func NewOrganizationService(pool *pgxpool.Pool) *OrganizationService {
	return &OrganizationService{pool: pool}
}

// roleRank prefers organization_admin when several roles apply to one org
// (DB assignment plus an IdP group binding, for example).
func roleRank(role string) int {
	switch role {
	case "organization_admin":
		return 100
	case "edition_organizer":
		return 80
	case "programme_manager":
		return 70
	case "reviewer":
		return 40
	case "submitter":
		return 20
	case "viewer":
		return 10
	default:
		return 0
	}
}

// ListVisible returns organizations the caller can see: DB memberships and
// IdP group bindings from access-config.toml (already resolved onto
// auth.User.Memberships). Earlier this only joined the memberships table, so
// staff who only had an events-* group saw an empty Organizations list while
// the sidebar still showed CfP and Schedule.
func (s *OrganizationService) ListVisible(ctx context.Context) ([]Organization, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if len(user.Memberships) == 0 {
		return []Organization{}, nil
	}

	bestRole := map[uuid.UUID]string{}
	ids := make([]uuid.UUID, 0, len(user.Memberships))
	seen := map[uuid.UUID]bool{}
	for _, m := range user.Memberships {
		if !seen[m.OrganizationID] {
			seen[m.OrganizationID] = true
			ids = append(ids, m.OrganizationID)
		}
		if prev, ok := bestRole[m.OrganizationID]; !ok || roleRank(m.Role) > roleRank(prev) {
			bestRole[m.OrganizationID] = m.Role
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, slug, display_name
		  FROM organizations
		 WHERE id = ANY($1)
		 ORDER BY display_name ASC
	`, ids)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()

	out := make([]Organization, 0, len(ids))
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Slug, &o.DisplayName); err != nil {
			return nil, httperror.Internal(err)
		}
		o.Role = bestRole[o.ID]
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].DisplayName < out[j].DisplayName
	})
	return out, nil
}

// Get returns a single organization the caller can see, or 404 otherwise.
func (s *OrganizationService) Get(ctx context.Context, id uuid.UUID) (*Organization, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	var role string
	ok := false
	for _, m := range user.Memberships {
		if m.OrganizationID == id {
			ok = true
			if roleRank(m.Role) > roleRank(role) {
				role = m.Role
			}
		}
	}
	if !ok {
		return nil, httperror.NotFound("organization")
	}
	var o Organization
	err = s.pool.QueryRow(ctx, `
		SELECT id, slug, display_name FROM organizations WHERE id = $1
	`, id).Scan(&o.ID, &o.Slug, &o.DisplayName)
	switch {
	case err == nil:
		o.Role = role
		return &o, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("organization")
	default:
		return nil, httperror.Internal(err)
	}
}
