package auth

import (
	"context"
	"reflect"
	"testing"

	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/groupmatch"
)

func staffConfig(f *fakeIssuer) *config.Config {
	return &config.Config{
		AuthMode:      config.AuthModeOIDC,
		OIDCIssuerURL: f.issuer("staff"),
		OIDCClientID:  "events-spa",
		OIDCAudience:  "events-api",
		Groups:        groupmatch.Config{Claim: "groups", Mode: groupmatch.ModeFull},
	}
}

// Groups come only from OIDC_GROUPS_CLAIM: a realm role in "roles" grants
// nothing.
func TestStaffGroupsOnlyFromConfiguredClaim(t *testing.T) {
	f := newFakeIssuer(t)
	ctx := context.Background()
	v, err := NewOIDCVerifier(ctx, staffConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	c, err := v.Verify(ctx, f.token("staff", map[string]any{
		"sub": "u1", "aud": "events-api",
		"groups": []string{"/events-reviewer"}, "roles": []string{"events-admin"},
	}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !reflect.DeepEqual(c.Groups, []string{"/events-reviewer"}) {
		t.Fatalf("Groups = %v, want only the groups claim", c.Groups)
	}

	cfg := staffConfig(f)
	cfg.Groups.Claim = "realm_access.roles"
	v, err = NewOIDCVerifier(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	c, err = v.Verify(ctx, f.token("staff", map[string]any{
		"sub": "u1", "aud": "events-api",
		"groups": []string{"/events-reviewer"}, "realm_access": map[string]any{"roles": []string{"events-admin"}},
	}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !reflect.DeepEqual(c.Groups, []string{"events-admin"}) {
		t.Fatalf("Groups = %v, want the nested realm_access.roles claim only", c.Groups)
	}
}

// Attendee tokens never carry groups, whatever the token says.
func TestAttendeeVerifierIgnoresGroups(t *testing.T) {
	f := newFakeIssuer(t)
	ctx := context.Background()
	v, err := NewOIDCVerifierFor(ctx, f.issuer("attendees"), "events-public-api")
	if err != nil {
		t.Fatal(err)
	}
	c, err := v.Verify(ctx, f.token("attendees", map[string]any{
		"sub": "a1", "aud": "events-public-api", "groups": []string{"/events-admin"},
	}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(c.Groups) != 0 {
		t.Fatalf("attendee Groups = %v, want none", c.Groups)
	}
}
