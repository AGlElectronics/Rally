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
	ResourceCallForPaper ResourceType = "call_for_papers"
	ResourceSubmission   ResourceType = "submission"
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
// more as the vertical slices (CfP → review → schedule → publish) land.
const (
	PermOrgView      = "org.view"
	PermOrgManage    = "org.manage"
	PermEditionView  = "edition.view"
	PermEditionEdit  = "edition.edit"
	PermMemberView   = "member.view"
	PermMemberManage = "member.manage"
	PermAuditView    = "audit.view"

	// CfP slice. cfp.manage creates/edits/deletes calls for papers;
	// submission.review lists every submission of an edition and
	// accepts/rejects them. Submitting a talk needs no permission beyond
	// authentication (an open CfP is open to any signed-in user).
	PermCfPManage        = "cfp.manage"
	PermSubmissionReview = "submission.review"
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
		PermCfPManage, PermSubmissionReview,
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
			PermCfPManage, PermSubmissionReview,
		},
		RoleProgrammeManager: {
			PermOrgView, PermEditionView, PermMemberView,
			PermSubmissionReview,
		},
		RoleReviewer: {
			PermOrgView, PermEditionView,
			PermSubmissionReview,
		},
		RoleSubmitter: {
			PermOrgView, PermEditionView,
		},
		RoleViewer: {
			PermOrgView, PermEditionView,
		},
	}
}
