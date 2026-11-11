package config

import (
	"strings"
	"testing"
)

// baseEnv is the smallest valid production environment.
func baseEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"APP_ENV":                  "",
		"AUTH_MODE":                "",
		"DATABASE_URL":             "postgres://rally@localhost/rally",
		"S3_ENABLED":               "false",
		"OIDC_ISSUER_URL":          "https://auth.example.org/realms/staff",
		"OIDC_CLIENT_ID":           "events-spa",
		"ATTENDEE_OIDC_ISSUER_URL": "",
		"DEN_LINK_ENCRYPTION_KEY":  "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"DEN_MODE":                 "",
		"PUBLIC_RATE_LIMIT":        "",
		"PUBLIC_RATE_BURST":        "",
		"SUBMISSION_RATE_LIMIT":    "",
		"SUBMISSION_RATE_BURST":    "",
		"OIDC_GROUPS_CLAIM":        "",
		"OIDC_GROUP_MATCH":         "",
		"OIDC_GROUP_PREFIX":        "",
		"HR_SERVICE_TOKEN":         "",
	} {
		t.Setenv(k, v)
	}
}

func TestLoadDefaultsToOIDCAndProduction(t *testing.T) {
	baseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuthMode != AuthModeOIDC || cfg.AppEnv != "production" {
		t.Fatalf("AuthMode=%q AppEnv=%q, want oidc and production", cfg.AuthMode, cfg.AppEnv)
	}
	if cfg.Den.AppEnv != "production" || cfg.Den.Slug != DenSlug {
		t.Fatalf("Den config = %+v", cfg.Den)
	}
	if cfg.PublicRateLimit.Rate != 2 || cfg.PublicRateLimit.Burst != DefaultPublicRateBurst {
		t.Fatalf("PublicRateLimit = %+v, want 120/m burst %d", cfg.PublicRateLimit, DefaultPublicRateBurst)
	}
	if cfg.SubmissionRateLimit.Burst != DefaultSubmissionRateBurst {
		t.Fatalf("SubmissionRateLimit = %+v", cfg.SubmissionRateLimit)
	}
}

func TestLoadRefusesDevModeOutsideDevelopment(t *testing.T) {
	for _, env := range []string{"", "production", "staging"} {
		baseEnv(t)
		t.Setenv("APP_ENV", env)
		t.Setenv("AUTH_MODE", "dev")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "only allowed with APP_ENV=development") {
			t.Errorf("APP_ENV=%q AUTH_MODE=dev: err = %v, want the authbase guard", env, err)
		}
	}
	baseEnv(t)
	t.Setenv("APP_ENV", "Development")
	t.Setenv("AUTH_MODE", "dev")
	t.Setenv("DEN_LINK_ENCRYPTION_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("APP_ENV=development AUTH_MODE=dev: %v", err)
	}
	if !cfg.DevAuth() || cfg.Den.AppEnv != "development" {
		t.Fatalf("DevAuth=%v Den.AppEnv=%q", cfg.DevAuth(), cfg.Den.AppEnv)
	}
}

func TestLoadRejectsUnknownAuthMode(t *testing.T) {
	baseEnv(t)
	t.Setenv("AUTH_MODE", "none")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "AUTH_MODE") {
		t.Fatalf("err = %v, want an AUTH_MODE error", err)
	}
}

func TestLoadRequiresDenKeyOutsideDevelopment(t *testing.T) {
	baseEnv(t)
	t.Setenv("DEN_LINK_ENCRYPTION_KEY", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DEN_LINK_ENCRYPTION_KEY") {
		t.Fatalf("err = %v, want DEN_LINK_ENCRYPTION_KEY required", err)
	}
	t.Setenv("DEN_LINK_ENCRYPTION_KEY", "not-a-key")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DEN_LINK_ENCRYPTION_KEY") {
		t.Fatalf("err = %v, want a bad-key error", err)
	}
}

func TestNormalizeIssuer(t *testing.T) {
	for in, want := range map[string]string{
		"https://auth.example.org/realms/staff":           "https://auth.example.org/realms/staff",
		" HTTPS://Auth.Example.org/realms/staff/ ":        "https://auth.example.org/realms/staff",
		"https://auth.example.org:443/realms/staff":       "https://auth.example.org/realms/staff",
		"http://localhost:8080/realms/Staff":              "http://localhost:8080/realms/Staff",
		"http://keycloak:80/realms/attendees//":           "http://keycloak/realms/attendees",
		"https://tenant.eu.auth0.com/":                    "https://tenant.eu.auth0.com",
		"https://auth.example.org/realms/staff?ignored=1": "https://auth.example.org/realms/staff",
	} {
		if got := NormalizeIssuer(in); got != want {
			t.Errorf("NormalizeIssuer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadRejectsAttendeeRealmEqualToStaff(t *testing.T) {
	baseEnv(t)
	t.Setenv("ATTENDEE_OIDC_ISSUER_URL", "https://AUTH.example.org:443/realms/staff/")
	t.Setenv("ATTENDEE_OIDC_CLIENT_ID", "rally-submit")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ATTENDEE_OIDC_ISSUER_URL must be a different realm") {
		t.Fatalf("err = %v, want the realm separation error", err)
	}
	t.Setenv("ATTENDEE_OIDC_ISSUER_URL", "https://auth.example.org/realms/attendees")
	if _, err := Load(); err != nil {
		t.Fatalf("separate realms: %v", err)
	}
}

func TestLoadHRServiceToken(t *testing.T) {
	baseEnv(t)
	t.Setenv("HR_SERVICE_TOKEN", "short")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "HR_SERVICE_TOKEN") {
		t.Fatalf("err = %v, want a length error", err)
	}
	t.Setenv("HR_SERVICE_TOKEN", " "+strings.Repeat("a", MinServiceTokenLength)+" ")
	cfg, err := Load()
	if err != nil || cfg.HRServiceToken != strings.Repeat("a", MinServiceTokenLength) {
		t.Fatalf("HRServiceToken = %q, %v", cfg.HRServiceToken, err)
	}
}

func TestLoadGroupMatching(t *testing.T) {
	baseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Groups.ClaimName() != "groups" || cfg.Groups.Mode != "full" {
		t.Fatalf("default Groups = %+v", cfg.Groups)
	}
	t.Setenv("OIDC_GROUPS_CLAIM", "realm_access.roles")
	t.Setenv("OIDC_GROUP_MATCH", "last")
	t.Setenv("OIDC_GROUP_PREFIX", "/con2026")
	if cfg, err = Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Groups.Claim != "realm_access.roles" || cfg.Groups.Mode != "last" || cfg.Groups.Prefix != "/con2026" {
		t.Fatalf("Groups = %+v", cfg.Groups)
	}
	t.Setenv("OIDC_GROUP_MATCH", "suffix")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OIDC_GROUP_MATCH") {
		t.Fatalf("err = %v, want an OIDC_GROUP_MATCH error", err)
	}
}

func TestLoadRateLimits(t *testing.T) {
	baseEnv(t)
	t.Setenv("PUBLIC_RATE_LIMIT", "off")
	t.Setenv("SUBMISSION_RATE_LIMIT", "6/h")
	t.Setenv("SUBMISSION_RATE_BURST", "3")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PublicRateLimit.Rate != 0 {
		t.Fatalf("PUBLIC_RATE_LIMIT=off gave %+v", cfg.PublicRateLimit)
	}
	if got := cfg.SubmissionRateLimit; got.Rate != 6.0/3600 || got.Burst != 3 {
		t.Fatalf("SubmissionRateLimit = %+v", got)
	}

	for k, v := range map[string]string{"PUBLIC_RATE_LIMIT": "fast", "PUBLIC_RATE_BURST": "0", "SUBMISSION_RATE_BURST": "many"} {
		baseEnv(t)
		t.Setenv(k, v)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), k) {
			t.Errorf("%s=%q: err = %v", k, v, err)
		}
	}
}
