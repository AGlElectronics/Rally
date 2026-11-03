package http

import (
	"net/http"
	"sort"
	"strings"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/domain"
)

type roleCatalogEntry struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Group       string   `json:"group"`
	Assignable  bool     `json:"assignable"`
	Scope       string   `json:"scope"`
	Permissions []string `json:"permissions"`
	Note        string   `json:"note,omitempty"`
}

var rallyRoleDescriptions = map[string]string{
	domain.RoleOrganizationAdmin: "Full control of the organization: editions, CfPs, members.",
	domain.RoleEditionOrganizer:  "Runs one edition: CfPs, submissions review, members.",
	domain.RoleProgrammeManager:  "Reviews and curates submissions for the programme.",
	domain.RoleReviewer:          "Reviews submissions assigned to them.",
	domain.RoleSubmitter:         "Submits talks to open calls for papers.",
	domain.RoleViewer:            "Read-only access to the organization and editions.",
}

// groupForRole lists roles that are granted from an IdP group. Others are
// per-edition role assignments managed inside Events itself.
var groupForRole = map[string]string{
	domain.RoleOrganizationAdmin: "events-admin",
}

// roleCatalogHandler serves GET /api/role-catalog (public, read-only).
func roleCatalogHandler(access *auth.AccessConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roles := make([]roleCatalogEntry, 0)
		for _, role := range access.Roles() {
			set := access.PermissionsForRole(role)
			perms := make([]string, 0, len(set))
			for p := range set {
				perms = append(perms, p)
			}
			sort.Strings(perms)
			entry := roleCatalogEntry{
				Key:         role,
				Label:       humanizeRole(role),
				Description: rallyRoleDescriptions[role],
				Scope:       "organization",
				Permissions: perms,
			}
			if g, ok := groupForRole[role]; ok {
				entry.Group = g
				entry.Assignable = true
			} else {
				entry.Scope = "edition"
				entry.Note = "Assigned per edition inside Events, not via an IdP group."
			}
			roles = append(roles, entry)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"module":       "events",
			"display_name": "Events",
			"description":  "Editions, calls for papers and the programme.",
			"roles":        roles,
		})
	}
}

func humanizeRole(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
