// Package auth manages the authenticated principal attached to each request
// and exposes RBAC helpers. In the scaffold it ships with a dev bypass
// (AUTH_MODE=dev) and a verified-OIDC path (AUTH_MODE=oidc); authorization
// decisions use the access config loaded at startup.
package auth

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
)

// Membership records that a user is a member of an organization and holds
// one role in it (optionally edition-scoped). A user may hold many
// memberships; permission checks iterate them.
type Membership struct {
	OrganizationID uuid.UUID
	EditionID      *uuid.UUID // nil = org-scoped
	Role           string
}

// User is the authenticated principal for a single HTTP request. It is
// constructed per-request by the Resolver and attached to the context.
//
// Permissions is a precomputed union of all org/edition-scoped permissions
// for cheap `HasAny` checks; precise organization-scoped and edition-scoped
// enforcement goes through HasInOrg / HasInEdition.
type User struct {
	ID          uuid.UUID
	Issuer      string
	Subject     string
	Email       string
	DisplayName string
	Memberships []Membership

	// Permissions holds the union of every role's permissions across every
	// membership; useful for coarse "is this feature visible?" checks.
	Permissions map[string]struct{}

	// OrgPermissions holds the permissions the user has when operating on
	// resources owned by a specific organization. It includes both org-level
	// and edition-level grants, projected to the parent org.
	OrgPermissions map[uuid.UUID]map[string]struct{}

	// EditionPermissions holds the permissions scoped to a specific edition.
	EditionPermissions map[uuid.UUID]map[string]struct{}

	// OrgWidePermissions holds only the permissions granted by org-scoped
	// role assignments (edition_id IS NULL). Unlike OrgPermissions it does
	// not include edition-scoped grants, so it is safe to apply to any
	// edition in that organization.
	OrgWidePermissions map[uuid.UUID]map[string]struct{}
}

// HasForEdition returns true if the user holds `perm` for the given edition,
// either through an org-wide grant on the owning organization or a grant
// scoped to that exact edition. This is the check edition-owned resources
// (CfPs, submissions, ...) should use; HasInOrg is too broad for them
// because it also counts grants for other editions.
func (u *User) HasForEdition(perm string, orgID, editionID uuid.UUID) bool {
	if u == nil {
		return false
	}
	if perms, ok := u.OrgWidePermissions[orgID]; ok {
		if _, ok := perms[perm]; ok {
			return true
		}
	}
	return u.HasInEdition(perm, editionID)
}

// HasAny returns true if the user holds `perm` anywhere (any org, any
// edition). Use for navigation gating; never for authorization decisions
// on a specific resource.
func (u *User) HasAny(perm string) bool {
	if u == nil {
		return false
	}
	_, ok := u.Permissions[perm]
	return ok
}

// HasInOrg returns true if the user holds `perm` for a specific
// organization (either org-scoped or via any edition-scoped assignment).
func (u *User) HasInOrg(perm string, orgID uuid.UUID) bool {
	if u == nil {
		return false
	}
	if perms, ok := u.OrgPermissions[orgID]; ok {
		if _, ok := perms[perm]; ok {
			return true
		}
	}
	return false
}

// HasInEdition returns true if the user holds `perm` for a specific
// edition. Prefer this over HasInOrg when the resource is edition-scoped.
func (u *User) HasInEdition(perm string, editionID uuid.UUID) bool {
	if u == nil {
		return false
	}
	if perms, ok := u.EditionPermissions[editionID]; ok {
		if _, ok := perms[perm]; ok {
			return true
		}
	}
	return false
}

// EffectivePermissions returns the sorted union of every grant. /api/v1/me
// exposes this so the SPA can gate navigation; precise enforcement always
// happens server-side via the HasIn* helpers.
func (u *User) EffectivePermissions() []string {
	if u == nil {
		return nil
	}
	out := make([]string, 0, len(u.Permissions))
	for p := range u.Permissions {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

type ctxKey int

const userKey ctxKey = 0

// WithUser stores the authenticated user on the context.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// FromContext returns the authenticated user, if any.
func FromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userKey).(*User)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}

// MustFromContext returns the user or an error if the request is
// unauthenticated. Handlers behind the auth middleware can rely on this.
func MustFromContext(ctx context.Context) (*User, error) {
	u, ok := FromContext(ctx)
	if !ok {
		return nil, errors.New("no authenticated user on context")
	}
	return u, nil
}

// ActorSnapshot captures identity metadata at request time for audit rows.
// Display names and emails can change in the IdP; (issuer, subject) is the
// durable external identity.
type ActorSnapshot struct {
	UserID      *uuid.UUID
	Issuer      string
	Subject     string
	DisplayName string
}

// ActorSnapshotFromContext builds a snapshot from the request's user, or an
// empty snapshot for unauthenticated contexts.
func ActorSnapshotFromContext(ctx context.Context) ActorSnapshot {
	u, ok := FromContext(ctx)
	if !ok {
		return ActorSnapshot{}
	}
	id := u.ID
	return ActorSnapshot{
		UserID:      &id,
		Issuer:      u.Issuer,
		Subject:     u.Subject,
		DisplayName: u.DisplayName,
	}
}
