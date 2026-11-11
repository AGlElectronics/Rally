package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

// Each realm's verifier refuses the other realm's tokens, and the
// middlewares say so.
func TestRealmSeparation(t *testing.T) {
	f := newFakeIssuer(t)
	ctx := context.Background()
	cfg := staffConfig(f)
	staff, err := NewOIDCVerifier(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	att, err := NewOIDCVerifierFor(ctx, f.issuer("attendees"), "events-api")
	if err != nil {
		t.Fatal(err)
	}
	// Same audience on purpose: only the realm tells the tokens apart.
	staffTok := f.token("staff", map[string]any{"sub": "s1", "aud": "events-api"})
	attTok := f.token("attendees", map[string]any{"sub": "a1", "aud": "events-api"})

	if err := SeparateRealms(staff, att); err != nil {
		t.Fatalf("SeparateRealms: %v", err)
	}
	if _, err := staff.Verify(ctx, attTok); !errors.Is(err, ErrOtherRealm) {
		t.Errorf("staff verifier, attendee token: %v, want ErrOtherRealm", err)
	}
	if _, err := att.Verify(ctx, staffTok); !errors.Is(err, ErrOtherRealm) {
		t.Errorf("attendee verifier, staff token: %v, want ErrOtherRealm", err)
	}
	// The comparison is on normalised issuers: a trailing slash or upper
	// case host does not get a token past it.
	odd := "HTTP" + strings.TrimPrefix(f.issuer("attendees"), "http") + "/"
	if _, err := staff.Verify(ctx, f.token("attendees", map[string]any{"iss": odd, "sub": "a1", "aud": "events-api"})); !errors.Is(err, ErrOtherRealm) {
		t.Errorf("staff verifier, attendee token with iss %q: %v, want ErrOtherRealm", odd, err)
	}
	if c, err := staff.Verify(ctx, staffTok); err != nil || c.Issuer != f.issuer("staff") {
		t.Errorf("staff token: %+v %v", c, err)
	}

	serve := func(mw func(http.Handler) http.Handler, tok string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })).ServeHTTP(rec, req)
		return rec
	}
	if rec := serve(Middleware(cfg, nil, staff), attTok); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "attendee accounts") {
		t.Errorf("staff API, attendee token: %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(AttendeeMiddleware(cfg, nil, att), staffTok); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "staff accounts") {
		t.Errorf("public API, staff token: %d %s", rec.Code, rec.Body.String())
	}

	same, err := NewOIDCVerifierFor(ctx, f.issuer("staff"), "events-public-api")
	if err != nil {
		t.Fatal(err)
	}
	if err := SeparateRealms(staff, same); err == nil {
		t.Errorf("SeparateRealms accepted the same issuer twice")
	}
	if err := SeparateRealms(staff, nil); err != nil {
		t.Errorf("SeparateRealms without attendee realm: %v", err)
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
