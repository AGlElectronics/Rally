package http

import (
	"net/http"

	"github.com/AGlElectronics/rally/backend/internal/auth"
)

// meHandler returns the authenticated user plus their memberships and the
// precomputed effective permissions. The SPA uses this on boot to gate
// navigation; precise authorization stays server-side.
func meHandler(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	memberships := make([]map[string]any, 0, len(u.Memberships))
	for _, m := range u.Memberships {
		var edition *string
		if m.EditionID != nil {
			s := m.EditionID.String()
			edition = &s
		}
		memberships = append(memberships, map[string]any{
			"organization_id": m.OrganizationID.String(),
			"edition_id":      edition,
			"role":            m.Role,
		})
	}
	perms := u.EffectivePermissions()
	if perms == nil {
		perms = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           u.ID.String(),
		"display_name": u.DisplayName,
		"email":        u.Email,
		"issuer":       u.Issuer,
		"subject":      u.Subject,
		"memberships":  memberships,
		"permissions":  perms,
	})
}
