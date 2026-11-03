package auth

import (
	"testing"

	"github.com/google/uuid"

	"github.com/AGlElectronics/rally/backend/internal/domain"
)

func TestHasForEdition(t *testing.T) {
	r := &Resolver{access: DefaultAccessConfig()}
	orgA, orgB := uuid.New(), uuid.New()
	edA1, edA2 := uuid.New(), uuid.New()

	// reviewer scoped to edA1 only; organizer org-wide on orgB.
	u := r.buildUser(uuid.New(), "dev", "x", "X", "x@x", []Membership{
		{OrganizationID: orgA, EditionID: &edA1, Role: domain.RoleReviewer},
		{OrganizationID: orgB, Role: domain.RoleEditionOrganizer},
	})

	if !u.HasForEdition(domain.PermSubmissionReview, orgA, edA1) {
		t.Error("edition-scoped reviewer should review its own edition")
	}
	if u.HasForEdition(domain.PermSubmissionReview, orgA, edA2) {
		t.Error("edition-scoped grant must not leak to a sibling edition")
	}
	if u.HasForEdition(domain.PermCfPManage, orgA, edA1) {
		t.Error("reviewer must not hold cfp.manage")
	}
	if !u.HasForEdition(domain.PermCfPManage, orgB, uuid.New()) {
		t.Error("org-wide organizer should manage any edition in its org")
	}
	if u.HasForEdition(domain.PermCfPManage, orgA, uuid.New()) {
		t.Error("grant on org B must not apply to org A")
	}
	var nilUser *User
	if nilUser.HasForEdition(domain.PermCfPManage, orgA, edA1) {
		t.Error("nil user has no permissions")
	}
}
