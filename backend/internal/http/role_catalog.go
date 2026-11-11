package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// roleCatalogEntry is one role of the 1.0 role catalog contract
// (docs/concepts/role-catalog.md in con-services).
type roleCatalogEntry struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	Group        string   `json:"group"`
	GroupDisplay string   `json:"group_display"`
	Assignable   bool     `json:"assignable"`
	Default      bool     `json:"default"`
	Scope        string   `json:"scope"`
	Privileged   bool     `json:"privileged"`
	Dynamic      bool     `json:"dynamic"`
	Permissions  []string `json:"permissions"`
	Note         string   `json:"note,omitempty"`
}

type roleCatalog struct {
	Module      string             `json:"module"`
	DisplayName string             `json:"display_name"`
	Description string             `json:"description"`
	Revision    string             `json:"revision"`
	Roles       []roleCatalogEntry `json:"roles"`
}

var rallyRoleDescriptions = map[string]string{
	domain.RoleOrganizationAdmin: "Full control of the organization: editions, CfPs, members.",
	domain.RoleEditionOrganizer:  "Runs one edition: CfPs, submissions review, members.",
	domain.RoleProgrammeManager:  "Reviews and curates submissions for the programme.",
	domain.RoleReviewer:          "Reviews submissions assigned to them.",
	domain.RoleSubmitter:         "Submits talks to open calls for papers.",
	domain.RoleViewer:            "Read-only access to the organization and editions.",
}

// buildRoleCatalog lists every role once. A role bound to an IdP group in
// the access config is assignable from HR through its first binding's
// group; the rest are per-edition assignments managed inside Rally. The
// catalog only changes with the access config, so it is built once and its
// revision is a hash of its content (the same on every replica).
func buildRoleCatalog(access *auth.AccessConfig) roleCatalog {
	roles := make([]roleCatalogEntry, 0)
	bindings := access.Bindings()
	for _, role := range access.Roles() {
		set := access.PermissionsForRole(role)
		perms := make([]string, 0, len(set))
		for p := range set {
			perms = append(perms, p)
		}
		sort.Strings(perms)
		desc := access.RoleDescription(role)
		if desc == "" {
			desc = rallyRoleDescriptions[role]
		}
		entry := roleCatalogEntry{
			Key:         role,
			Label:       humanizeRole(role),
			Description: desc,
			Scope:       "edition",
			Privileged:  access.Privileged(role),
			Permissions: perms,
			Note:        "Assigned per edition inside Rally, not via an IdP group.",
		}
		for _, b := range bindings {
			if b.Role == role {
				entry.Group, entry.GroupDisplay = b.Group.Stored, b.Group.Display
				entry.Assignable, entry.Scope, entry.Note = true, "organization", ""
				break
			}
		}
		roles = append(roles, entry)
	}
	c := roleCatalog{
		Module:      "events",
		DisplayName: "Events",
		Description: "Editions, calls for papers and the programme.",
		Roles:       roles,
	}
	body, _ := json.Marshal(c)
	sum := sha256.Sum256(body)
	c.Revision = "sha256-" + hex.EncodeToString(sum[:8])
	return c
}

// roleCatalogHandler serves GET /api/role-catalog. The revision doubles as
// the ETag, so HR can revalidate with If-None-Match.
func roleCatalogHandler(access *auth.AccessConfig) http.HandlerFunc {
	catalog := buildRoleCatalog(access)
	etag := `"` + catalog.Revision + `"`
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "private, no-cache")
		if match := r.Header.Get("If-None-Match"); match != "" && (match == etag || match == "*") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		writeJSON(w, http.StatusOK, catalog)
	}
}

// requireCatalogReader lets HR's backend in with HR_SERVICE_TOKEN (compared
// in constant time; it allows nothing else) and everyone else through the
// staff sign-in (any role, or none). Attendee tokens fail the staff check.
func requireCatalogReader(serviceToken string, devAuth bool, staff func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		viaStaff := staff(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bearer := ""
			if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
				bearer = strings.TrimSpace(h[7:])
			}
			if bearer == "" && !devAuth {
				httperror.Render(w, r, httperror.ErrUnauthorized)
				return
			}
			if serviceToken != "" && subtle.ConstantTimeCompare([]byte(bearer), []byte(serviceToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			viaStaff.ServeHTTP(w, r)
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
