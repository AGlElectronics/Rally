package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/denlink"
	"github.com/AGlElectronics/rally/backend/internal/groupmatch"
	"github.com/AGlElectronics/rally/backend/internal/oidctest"
)

const testHRToken = "hr-service-token-for-tests-0123456789abcdef"

func catalogRouter(t *testing.T, pool *pgxpool.Pool) (http.Handler, *oidctest.Issuer) {
	t.Helper()
	idp := oidctest.New(t)
	ctx := context.Background()
	cfg := &config.Config{
		AuthMode:              config.AuthModeOIDC,
		AppEnv:                "production",
		OIDCIssuerURL:         idp.URL("staff"),
		OIDCClientID:          "events-spa",
		OIDCAudience:          "events-api",
		AttendeeOIDCIssuerURL: idp.URL("attendees"),
		HRServiceToken:        testHRToken,
		Groups:                groupmatch.Config{},
		Den:                   denlink.Config{Mode: denlink.ModeRequired},
	}
	staff, err := auth.NewOIDCVerifier(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	att, err := auth.NewOIDCVerifierFor(ctx, cfg.AttendeeOIDCIssuerURL, "events-public-api")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SeparateRealms(staff, att); err != nil {
		t.Fatal(err)
	}
	access, err := auth.LoadAccessConfig("../../../infra/access-config.toml")
	if err != nil {
		t.Fatal(err)
	}
	den := denlink.New(nil, denlink.Config{Slug: "test", Mode: denlink.ModeRequired, AppEnv: "development"}, zerolog.Nop())
	return NewRouter(Deps{Config: cfg, Pool: pool, Logger: zerolog.Nop(), Den: den, Access: access,
		OIDCVerifier: staff, AttendeeVerifier: att}, "test"), idp
}

func getCatalog(router http.Handler, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/role-catalog", nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRoleCatalogNeedsServiceTokenOrStaff(t *testing.T) {
	router, idp := catalogRouter(t, nil)

	rec := getCatalog(router, nil)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"authentication required"`) {
		t.Fatalf("no token: %d %s", rec.Code, rec.Body.String())
	}
	for name, bearer := range map[string]string{
		"wrong service token":         "hr-service-token-for-tests-0123456789abcdeX",
		"attendee token":              idp.Token("attendees", map[string]any{"sub": "a1", "aud": "events-public-api"}),
		"staff token for another API": idp.Token("staff", map[string]any{"sub": "s1", "aud": "hr-api"}),
	} {
		if rec := getCatalog(router, map[string]string{"Authorization": "Bearer " + bearer}); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d %s, want 401", name, rec.Code, rec.Body.String())
		}
	}

	// HR's service token works, also while Rally waits for Den.
	rec = getCatalog(router, map[string]string{"Authorization": "Bearer " + testHRToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("service token: %d %s", rec.Code, rec.Body.String())
	}
	var cat roleCatalog
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	if cat.Module != "events" || !strings.HasPrefix(cat.Revision, "sha256-") || rec.Header().Get("ETag") != `"`+cat.Revision+`"` {
		t.Fatalf("catalog header: module=%q revision=%q ETag=%q", cat.Module, cat.Revision, rec.Header().Get("ETag"))
	}
	byKey := map[string]roleCatalogEntry{}
	for _, r := range cat.Roles {
		if _, dup := byKey[r.Key]; dup {
			t.Fatalf("role %q listed twice", r.Key)
		}
		byKey[r.Key] = r
	}
	admin := byKey["organization_admin"]
	if admin.Group != "events-admin" || admin.GroupDisplay != "events-admin" || !admin.Assignable || admin.Scope != "organization" || !admin.Privileged || admin.Dynamic {
		t.Errorf("organization_admin = %+v", admin)
	}
	if r := byKey["reviewer"]; !r.Privileged || !r.Assignable {
		t.Errorf("reviewer sees speakers' submissions, must be privileged: %+v", r)
	}
	if v := byKey["viewer"]; v.Privileged || v.Assignable || v.Group != "" || v.Scope != "edition" {
		t.Errorf("viewer = %+v", v)
	}

	// Revalidation with the ETag.
	rec = getCatalog(router, map[string]string{"Authorization": "Bearer " + testHRToken, "If-None-Match": `"` + cat.Revision + `"`})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("If-None-Match: %d %q", rec.Code, rec.Body.String())
	}
}

func TestRoleCatalogStaffSignIn(t *testing.T) {
	dsn := os.Getenv("RALLY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RALLY_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	router, idp := catalogRouter(t, pool)
	// A staff member without any role may read it: it names roles, not people.
	tok := idp.Token("staff", map[string]any{"sub": "catalog-reader", "aud": "events-api", "name": "Catalog Reader"})
	if rec := getCatalog(router, map[string]string{"Authorization": "Bearer " + tok}); rec.Code != http.StatusOK {
		t.Fatalf("staff token: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPrivilegedOverride(t *testing.T) {
	access, err := auth.LoadAccessConfig("../../../infra/access-config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if access.Privileged("viewer") || access.Privileged("submitter") || !access.Privileged("programme_manager") {
		t.Fatalf("default privileged flags wrong")
	}
	f, err := os.CreateTemp(t.TempDir(), "access-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("[roles.viewer]\npermissions = [\"org.view\"]\nprivileged = true\n[roles.reviewer]\npermissions = [\"submission.review\"]\nprivileged = false\n")
	_ = f.Close()
	access, err = auth.LoadAccessConfig(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !access.Privileged("viewer") || access.Privileged("reviewer") {
		t.Fatalf("privileged overrides not applied")
	}
}
