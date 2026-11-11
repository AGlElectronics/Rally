package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/groupmatch"
)

// Resolver turns an authenticated external identity (OIDC claims, or a dev
// bypass header) into a *User backed by Rally's users table.
//
// Identity invariants (architecture.md §4.4):
//
//   - The (issuer, subject) pair is the stable identity key. Email and
//     display name are refreshed every request; never used as join keys.
//   - A user existing with the same email as an incoming OIDC login is
//     NEVER automatically linked. The scaffold always creates a new local
//     user for a never-seen-before (issuer, subject) pair; deliberate
//     linking is a later migration workflow.
type Resolver struct {
	pool   *pgxpool.Pool
	access *AccessConfig
	// match compares token groups with the access config bindings.
	match  groupmatch.Config
	orgIDs sync.Map // organization slug -> uuid.UUID
}

// NewResolver constructs a Resolver backed by the given pool, access config
// catalog and group matching rule (OIDC_GROUP_MATCH, OIDC_GROUP_PREFIX).
func NewResolver(pool *pgxpool.Pool, access *AccessConfig, match groupmatch.Config) *Resolver {
	return &Resolver{pool: pool, access: access, match: match}
}

// LoadOrProvisionDevUser is used by the dev bypass middleware. The header
// becomes the OIDC subject under the reserved "dev" issuer. Dev users are
// always granted the organization_admin role on the demo org so local
// scaffolding is unblocked without manual seeding.
func (r *Resolver) LoadOrProvisionDevUser(ctx context.Context, headerValue string) (*User, error) {
	headerValue = strings.TrimSpace(headerValue)
	if headerValue == "" {
		headerValue = "dev-admin"
	}
	issuer := "dev"
	subject := headerValue

	display := displayFromDevHeader(headerValue)
	email := headerValue + "@rally.local"

	userID, err := r.upsertUserByExternal(ctx, issuer, subject, display, &email)
	if err != nil {
		return nil, fmt.Errorf("provision dev user: %w", err)
	}

	// Load the real memberships so the dev user still demonstrates the
	// authorization paths; the init migration seeds organization_admin on
	// the demo org for this subject.
	memberships, err := r.loadMemberships(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load dev memberships: %w", err)
	}

	return r.buildUser(userID, issuer, subject, display, email, memberships), nil
}

// LoadOrProvisionOIDCUser is called by the OIDC middleware after a
// successful JWT verification. It upserts the identity row, refreshes the
// user metadata (display name, email, last login), then returns the request
// principal populated from the access config catalog.
func (r *Resolver) LoadOrProvisionOIDCUser(ctx context.Context, claims *OIDCClaims) (*User, error) {
	if claims == nil || claims.Subject == "" || claims.Issuer == "" {
		return nil, errors.New("oidc claims missing issuer/subject")
	}
	display := displayNameFromClaims(claims)
	var emailPtr *string
	if e := strings.TrimSpace(claims.Email); e != "" {
		emailPtr = &e
	}
	userID, err := r.upsertUserByExternal(ctx, claims.Issuer, claims.Subject, display, emailPtr)
	if err != nil {
		return nil, fmt.Errorf("upsert oidc user: %w", err)
	}

	memberships, err := r.loadMemberships(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load memberships: %w", err)
	}
	groupRoles, err := r.groupMemberships(ctx, claims.Groups)
	if err != nil {
		return nil, fmt.Errorf("resolve group bindings: %w", err)
	}
	memberships = append(memberships, groupRoles...)

	email := ""
	if emailPtr != nil {
		email = *emailPtr
	}
	return r.buildUser(userID, claims.Issuer, claims.Subject, display, email, memberships), nil
}

// upsertUserByExternal looks up the local user by (issuer, subject). If
// absent, a fresh users row is created and bound to the pair. Metadata
// (display_name, email, last_login_at) is refreshed on every call.
func (r *Resolver) upsertUserByExternal(
	ctx context.Context,
	issuer, subject, display string,
	email *string,
) (uuid.UUID, error) {
	var userID uuid.UUID

	err := r.pool.QueryRow(ctx,
		`SELECT user_id FROM external_identities WHERE issuer = $1 AND subject = $2`,
		issuer, subject,
	).Scan(&userID)
	switch {
	case err == nil:
		// Known identity — refresh metadata.
		_, err = r.pool.Exec(ctx,
			`UPDATE users
			    SET display_name  = $2,
			        email         = COALESCE($3, email),
			        last_login_at = now(),
			        updated_at    = now()
			  WHERE id = $1`,
			userID, display, email,
		)
		return userID, err
	case errors.Is(err, pgx.ErrNoRows):
		// New identity — create user and binding atomically.
		tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return uuid.Nil, err
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if err := tx.QueryRow(ctx,
			`INSERT INTO users (display_name, email, last_login_at)
			 VALUES ($1, $2, now())
			 RETURNING id`,
			display, email,
		).Scan(&userID); err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO external_identities (user_id, issuer, subject)
			 VALUES ($1, $2, $3)`,
			userID, issuer, subject,
		); err != nil {
			return uuid.Nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return uuid.Nil, err
		}
		return userID, nil
	default:
		return uuid.Nil, err
	}
}

// loadMemberships returns every active organization + role grant for a
// user. Rows are flattened: one Membership per (organization, edition, role)
// tuple, so permission projection downstream is just a loop.
func (r *Resolver) loadMemberships(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	// Only "active" memberships count. Role assignments where the
	// corresponding membership is not active are ignored even if the row
	// exists, so suspending a member revokes authority without deleting
	// their history.
	rows, err := r.pool.Query(ctx, `
		SELECT ra.organization_id, ra.edition_id, ra.role
		  FROM role_assignments ra
		  JOIN memberships m
		    ON m.organization_id = ra.organization_id
		   AND m.user_id         = ra.user_id
		 WHERE ra.user_id = $1
		   AND m.status   = 'active'
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.OrganizationID, &m.EditionID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// buildUser assembles the request principal from the identity row and
// loaded memberships, projecting role → permission grants from the access
// config catalog.
func (r *Resolver) buildUser(
	userID uuid.UUID,
	issuer, subject, display, email string,
	memberships []Membership,
) *User {
	global := map[string]struct{}{}
	orgPerms := map[uuid.UUID]map[string]struct{}{}
	editionPerms := map[uuid.UUID]map[string]struct{}{}
	orgWide := map[uuid.UUID]map[string]struct{}{}

	for _, m := range memberships {
		set := r.access.PermissionsForRole(m.Role)
		if set == nil {
			continue
		}
		orgSet, ok := orgPerms[m.OrganizationID]
		if !ok {
			orgSet = map[string]struct{}{}
			orgPerms[m.OrganizationID] = orgSet
		}
		for p := range set {
			global[p] = struct{}{}
			orgSet[p] = struct{}{}
			if m.EditionID == nil {
				wide, ok := orgWide[m.OrganizationID]
				if !ok {
					wide = map[string]struct{}{}
					orgWide[m.OrganizationID] = wide
				}
				wide[p] = struct{}{}
			}
			if m.EditionID != nil {
				edSet, ok := editionPerms[*m.EditionID]
				if !ok {
					edSet = map[string]struct{}{}
					editionPerms[*m.EditionID] = edSet
				}
				edSet[p] = struct{}{}
			}
		}
	}

	return &User{
		ID:                 userID,
		Issuer:             issuer,
		Subject:            subject,
		Email:              email,
		DisplayName:        display,
		Memberships:        memberships,
		Permissions:        global,
		OrgPermissions:     orgPerms,
		EditionPermissions: editionPerms,
		OrgWidePermissions: orgWide,
	}
}

func displayFromDevHeader(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return "Dev User"
	}
	// "dev-admin" → "Dev Admin". Small cosmetic nicety; the identity key
	// stays the raw header value.
	parts := strings.Split(h, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func displayNameFromClaims(c *OIDCClaims) string {
	if c == nil {
		return ""
	}
	for _, s := range []string{c.Nickname, c.Name, c.PreferredUsername, c.Email} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	// Fallback to the subject — never pretty, but always non-empty.
	return c.Subject
}

// groupMemberships turns the token's IdP groups into organization-wide
// memberships via the access config bindings. Nothing is persisted, so
// removing someone from a group revokes the role on their next request.
// Bindings naming an organization slug that does not exist grant nothing.
func (r *Resolver) groupMemberships(ctx context.Context, groups []string) ([]Membership, error) {
	var out []Membership
	for _, b := range r.access.BindingsFor(r.match, groups) {
		orgID, ok, err := r.orgIDBySlug(ctx, b.Organization)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, Membership{OrganizationID: orgID, Role: b.Role})
		}
	}
	return out, nil
}

func (r *Resolver) orgIDBySlug(ctx context.Context, slug string) (uuid.UUID, bool, error) {
	if id, ok := r.orgIDs.Load(slug); ok {
		return id.(uuid.UUID), true, nil
	}
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = $1`, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	r.orgIDs.Store(slug, id)
	return id, true, nil
}

// Ensure referenced symbol from domain is used elsewhere; this prevents
// `go vet` from flagging an unused import if the scaffold happens not to
// touch domain constants directly in this file.
var _ = domain.RoleOrganizationAdmin
