// Package domain holds the neutral events vocabulary — entity types,
// role names, and permission identifiers — used by both the service layer
// and the HTTP layer. Keeping these as typed constants prevents stringly
// typed drift between tables, APIs, and the access config file.
package domain

// ResourceType names the auditable kinds of resources Rally manages. New
// entries belong here (not in call sites) so the audit log remains a closed
// vocabulary.
type ResourceType string

const (
	ResourceOrganization ResourceType = "organization"
	ResourceConvention   ResourceType = "convention"
	ResourceEdition      ResourceType = "edition"
	ResourceUser         ResourceType = "user"
	ResourceMembership   ResourceType = "membership"
	ResourceRole         ResourceType = "role_assignment"
)

// Role names recognized by default. The access config file (TOML) may add
// more roles or redefine the permission set attached to any of these.
const (
	RoleOrganizationAdmin = "organization_admin"
	RoleEditionOrganizer  = "edition_organizer"
	RoleProgrammeManager  = "programme_manager"
	RoleReviewer          = "reviewer"
	RoleSubmitter         = "submitter"
	RoleViewer            = "viewer"
)

// Permission identifiers. The initial set is intentionally minimal — we add
// more as the first vertical slice (CfP → review → schedule → publish) lands.
const (
	PermOrgView      = "org.view"
	PermOrgManage    = "org.manage"
	PermEditionView  = "edition.view"
	PermEditionEdit  = "edition.edit"
	PermMemberView   = "member.view"
	PermMemberManage = "member.manage"
	PermAuditView    = "audit.view"
)

// AllPermissions returns every permission the access-config validator will
// accept. The access config file may only reference identifiers listed here;
// unknown names fail configuration load with a clear error.
func AllPermissions() []string {
	return []string{
		PermOrgView, PermOrgManage,
		PermEditionView, PermEditionEdit,
		PermMemberView, PermMemberManage,
		PermAuditView,
	}
}

// DefaultRolePermissions is the built-in role → permission catalog applied
// when the access config file omits a role (or isn't configured at all).
// The admin role gets everything; others get the smallest useful set.
func DefaultRolePermissions() map[string][]string {
	return map[string][]string{
		RoleOrganizationAdmin: AllPermissions(),
		RoleEditionOrganizer: {
			PermOrgView,
			PermEditionView, PermEditionEdit,
			PermMemberView, PermAuditView,
		},
		RoleProgrammeManager: {
			PermOrgView, PermEditionView, PermMemberView,
		},
		RoleReviewer: {
			PermOrgView, PermEditionView,
		},
		RoleSubmitter: {
			PermOrgView, PermEditionView,
		},
		RoleViewer: {
			PermOrgView, PermEditionView,
		},
	}
}
