package auth

import (
	"fmt"
	"os"
	"sort"

	"github.com/AGlElectronics/rally/backend/internal/domain"
)

// AccessConfig is the in-memory snapshot of the role/permission catalog.
// It is immutable after construction and safe for concurrent reads.
//
// For the scaffold the config file is a plain Go-readable JSON (one small
// file, no extra deps). The Stash reference uses TOML; swap to TOML here if
// the operator preference lands on that. The important invariant is: the
// catalog is loaded ONCE at startup and never mutated — role revocation is
// a configuration-and-restart operation, not a hot write.
type AccessConfig struct {
	rolePerms map[string]map[string]struct{}
}

// DefaultAccessConfig returns the built-in role catalog. Used in dev mode
// when no access config file is set.
func DefaultAccessConfig() *AccessConfig {
	return &AccessConfig{rolePerms: defaultRolePermSets()}
}

// PermissionsForRole returns the permission set for a role or nil if the
// role is unknown. The returned map MUST NOT be mutated.
func (c *AccessConfig) PermissionsForRole(role string) map[string]struct{} {
	if c == nil {
		return nil
	}
	return c.rolePerms[role]
}

// HasRole reports whether the catalog knows about `role`.
func (c *AccessConfig) HasRole(role string) bool {
	if c == nil {
		return false
	}
	_, ok := c.rolePerms[role]
	return ok
}

// Roles returns the sorted role names in the catalog (for debug/admin APIs).
func (c *AccessConfig) Roles() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.rolePerms))
	for name := range c.rolePerms {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// LoadAccessConfig is a stub for the TOML/JSON loader. The scaffold does not
// yet parse the file; the plumbing exists so the first vertical slice can
// drop a parser in without touching callers.
//
// Returning the default catalog here keeps the scaffold runnable with only
// an environment variable set, while making the deliberate omission obvious
// via the warning log in app.Run.
func LoadAccessConfig(path string) (*AccessConfig, error) {
	// Minimal check: the file must exist, so a typo in ACCESS_CONFIG_PATH
	// surfaces at startup rather than silently falling back to defaults.
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("access config %q: %w", path, err)
	}
	// TODO(rally): parse the file (see docs/adr/0001-initial-scaffold.md).
	// For now, return defaults so the scaffold boots.
	return DefaultAccessConfig(), nil
}

func defaultRolePermSets() map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for role, perms := range domain.DefaultRolePermissions() {
		set := make(map[string]struct{}, len(perms))
		for _, p := range perms {
			set[p] = struct{}{}
		}
		out[role] = set
	}
	return out
}
