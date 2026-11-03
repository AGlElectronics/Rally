package auth

import (
	"os"
	"strings"
	"testing"
)

func TestShippedAccessConfigLoads(t *testing.T) {
	cfg, err := LoadAccessConfig("../../../infra/access-config.toml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := cfg.PermissionsForRole("organization_admin")["cfp.manage"]; !ok {
		t.Fatal("organization_admin lost cfp.manage")
	}
	got := cfg.BindingsFor([]string{"/events-admin"})
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
	if b := cfg.BindingsFor([]string{"events-helpers"}); len(b) != 1 || b[0].Organization != "main" {
		t.Fatalf("case-insensitive default-org binding: %+v", b)
	}
	if b := cfg.BindingsFor([]string{"events-admin"}); len(b) != 1 || b[0].Organization != "other" {
		t.Fatalf("per-binding organization: %+v", b)
	}
	if b := cfg.BindingsFor([]string{"someone-else"}); len(b) != 0 {
		t.Fatalf("unexpected bindings: %+v", b)
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
