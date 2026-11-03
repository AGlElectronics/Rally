package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// RequireAny verifies the user holds `perm` somewhere (any org/edition).
// Use it for navigation / list routes that filter server-side by scope.
func RequireAny(ctx context.Context, perm string) error {
	u, ok := FromContext(ctx)
	if !ok {
		return httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if u.HasAny(perm) {
		return nil
	}
	return httperror.New(httperror.CodeForbidden, "missing permission: "+perm)
}

// RequireInOrg enforces `perm` within a specific organization scope.
// Resource handlers resolve the owning org from the resource row (never the
// request body) and pass it here.
func RequireInOrg(ctx context.Context, perm string, orgID uuid.UUID) error {
	u, ok := FromContext(ctx)
	if !ok {
		return httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if u.HasInOrg(perm, orgID) {
		return nil
	}
	return httperror.New(httperror.CodeForbidden, "missing permission: "+perm)
}

// RequireForEdition enforces `perm` for an edition-owned resource: an
// org-wide grant on orgID or a grant scoped to editionID satisfies it.
func RequireForEdition(ctx context.Context, perm string, orgID, editionID uuid.UUID) error {
	u, ok := FromContext(ctx)
	if !ok {
		return httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if u.HasForEdition(perm, orgID, editionID) {
		return nil
	}
	return httperror.New(httperror.CodeForbidden, "missing permission: "+perm)
}

// RequireInEdition enforces `perm` within a specific edition scope.
func RequireInEdition(ctx context.Context, perm string, editionID uuid.UUID) error {
	u, ok := FromContext(ctx)
	if !ok {
		return httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if u.HasInEdition(perm, editionID) {
		return nil
	}
	return httperror.New(httperror.CodeForbidden, "missing permission: "+perm)
}
