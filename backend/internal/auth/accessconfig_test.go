package auth

import (
	"os"
	"strings"
	"testing"

	"github.com/AGlElectronics/rally/backend/internal/groupmatch"
)

func TestShippedAccessConfigLoads(t *testing.T) {
	cfg, err := LoadAccessConfig("../../../infra/access-config.toml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := cfg.PermissionsForRole("organization_admin")["cfp.manage"]; !ok {
		t.Fatal("organization_admin lost cfp.manage")
	}
	got := cfg.BindingsFor(groupmatch.Config{}, []string{"/events-admin"})
	if len(got) != 1 || got[0].Role != "organization_admin" || got[0].Organization != "demo-org" {
		t.Fatalf("events-admin binding: %+v", got)
	}
}

func TestParseAccessConfig(t *testing.T) {
	cfg, err := parseAccessConfig([]byte(`
organization = "main"
[roles.helper]
description = "Helps"
permissions = ["org.view"]
[[bindings]]
group = "Events-Helpers"
role = "helper"
[[bindings]]
group = "events-admin"
role = "organization_admin"
organization = "other"
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.RoleDescription("helper") != "Helps" {
		t.Fatalf("description: %q", cfg.RoleDescription("helper"))
	}
	full := groupmatch.Config{}
	if b := cfg.BindingsFor(full, []string{"events-helpers"}); len(b) != 1 || b[0].Organization != "main" {
		t.Fatalf("case-insensitive default-org binding: %+v", b)
	}
	if b := cfg.BindingsFor(full, []string{"events-admin"}); len(b) != 1 || b[0].Organization != "other" {
		t.Fatalf("per-binding organization: %+v", b)
	}
	if b := cfg.BindingsFor(full, []string{"someone-else"}); len(b) != 0 {
		t.Fatalf("unexpected bindings: %+v", b)
	}
	if b := cfg.Bindings()[0].Group; b.Stored != "events-helpers" || b.Display != "Events-Helpers" {
		t.Fatalf("grant = %+v, want stored normalised and display as written", b)
	}
}

// Bindings follow the shared rule: full paths by default, the last segment
// with OIDC_GROUP_MATCH=last, and OIDC_GROUP_PREFIX removed.
func TestBindingsForGroupMatchModes(t *testing.T) {
	cfg, err := parseAccessConfig([]byte(`
organization = "main"
[[bindings]]
group = "/departments/Events"
role = "edition_organizer"
[[bindings]]
group = "events-admin"
role = "organization_admin"
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	roles := func(m groupmatch.Config, groups ...string) []string {
		var out []string
		for _, b := range cfg.BindingsFor(m, groups) {
			out = append(out, b.Role)
		}
		return out
	}
	full := groupmatch.Config{}
	last := groupmatch.Config{Mode: groupmatch.ModeLast}
	prefixed := groupmatch.Config{Prefix: "/con2026"}

	if got := roles(full, "/departments/events"); len(got) != 1 {
		t.Errorf("full, same path: %v", got)
	}
	if got := roles(full, "/teams/events"); len(got) != 0 {
		t.Errorf("full, other path: %v", got)
	}
	if got := roles(last, "/teams/events"); len(got) != 1 {
		t.Errorf("last, same segment: %v", got)
	}
	if got := roles(prefixed, "/con2026/events-admin"); len(got) != 1 || got[0] != "organization_admin" {
		t.Errorf("prefix removed: %v", got)
	}
	if got := roles(prefixed, "/con2025/events-admin"); len(got) != 0 {
		t.Errorf("other year's parent: %v", got)
	}
}

func TestParseAccessConfigRejects(t *testing.T) {
	cases := map[string]string{
		"unknown permission": "[roles.x]\npermissions = [\"nope\"]",
		"unknown role":       "organization = \"o\"\n[[bindings]]\ngroup = \"g\"\nrole = \"ghost\"",
		"missing org":        "[[bindings]]\ngroup = \"g\"\nrole = \"viewer\"",
		"missing group":      "organization = \"o\"\n[[bindings]]\nrole = \"viewer\"",
	}
	for name, body := range cases {
		if _, err := parseAccessConfig([]byte(body)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoadAccessConfigMissingFile(t *testing.T) {
	_, err := LoadAccessConfig(os.TempDir() + "/does-not-exist.toml")
	if err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("expected file error, got %v", err)
	}
}
