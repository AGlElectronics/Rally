package http_test

// Public schedule API keys: the endpoints demand a key, keys are managed
// by publishing staff, shown once, revocable, and the builtin
// "programme-page" key follows PUBLIC_API_BUILTIN_KEY (EVT-41's missing
// coverage for the public schedule plus the new auth).
import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/brand"
	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/events"
	rallyhttp "github.com/AGlElectronics/rally/backend/internal/http"
)

func TestPublicScheduleAPIKeys(t *testing.T) {
	dsn := os.Getenv("RALLY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RALLY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	// Leftovers from earlier runs would break the "no key = 401" checks.
	if _, err := pool.Exec(ctx, `DELETE FROM public_api_keys`); err != nil {
		t.Fatalf("clean keys: %v", err)
	}

	keySvc := events.NewKeyService(pool)
	if err := keySvc.EnsureBuiltin(ctx, "rsk_builtin_test_value"); err != nil {
		t.Fatalf("ensure builtin: %v", err)
	}

	cfg := &config.Config{
		AppEnv:                "development",
		AppBaseURL:            "http://localhost:8100",
		HTTPAddr:              ":0",
		AuthMode:              config.AuthModeDev,
		DevDefaultUser:        "dev-admin",
		CORSAllowedOrigins:    []string{"*"},
		PublicScheduleEnabled: true,
	}
	router := rallyhttp.NewRouter(rallyhttp.Deps{
		Config: cfg,
		Pool:   pool,
		Logger: zerolog.Nop(),
		Brand:  brand.New(cfg),
		Access: auth.DefaultAccessConfig(),
		Keys:   keySvc,
	}, "test")

	do := func(user, method, path string, body any) *httptest.ResponseRecorder {
		var buf = bytes.NewReader(nil)
		if body != nil {
			b, _ := json.Marshal(body)
			buf = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, buf)
		if user != "" {
			req.Header.Set("X-Dev-User", user)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	anon := func(path, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// Without a key the public schedule answers 401 and says how to auth.
	rec := anon("/api/public/v1/schedules", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no key: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("401 must carry WWW-Authenticate")
	}
	if rec := anon("/api/public/v1/schedules", "rsk_wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", rec.Code)
	}

	// The builtin key (what the Programme page sends) works, via header…
	if rec := anon("/api/public/v1/schedules", "rsk_builtin_test_value"); rec.Code != http.StatusOK {
		t.Fatalf("builtin key via header: %d %s", rec.Code, rec.Body.String())
	}
	// …and via ?key= (calendar clients cannot send headers); the response
	// is private-cacheable now.
	rec = anon("/api/public/v1/schedules?key=rsk_builtin_test_value", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("builtin key via query: %d %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=60" {
		t.Fatalf("Cache-Control = %q", cc)
	}

	// Key management needs schedule.publish: alice (no roles) is refused.
	if rec := do("alice", http.MethodGet, "/api/v1/public-api-keys/", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("alice lists keys: %d %s", rec.Code, rec.Body.String())
	}

	// dev-admin creates a key for the website; the token appears once.
	rec = do("dev-admin", http.MethodPost, "/api/v1/public-api-keys/", map[string]any{"name": "Main website"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Key   struct{ ID string }
		Token string
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Token == "" || created.Key.ID == "" {
		t.Fatalf("create response: %s", rec.Body.String())
	}
	if rec := anon("/api/public/v1/schedules", created.Token); rec.Code != http.StatusOK {
		t.Fatalf("fresh key: %d", rec.Code)
	}

	// The listing shows metadata (with last_used_at) but never tokens.
	rec = do("dev-admin", http.MethodGet, "/api/v1/public-api-keys/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list keys: %d", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(created.Token)) {
		t.Fatalf("the token must never be listed")
	}
	var listing struct {
		Items []struct {
			Name       string  `json:"name"`
			Builtin    bool    `json:"builtin"`
			LastUsedAt *string `json:"last_used_at"`
		} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listing)
	if len(listing.Items) != 2 {
		t.Fatalf("expected the builtin + website keys, got %s", rec.Body.String())
	}

	// Revocation cuts access; the builtin key refuses API revocation (it
	// rotates through config instead).
	if rec := do("dev-admin", http.MethodDelete, "/api/v1/public-api-keys/"+created.Key.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	if rec := anon("/api/public/v1/schedules", created.Token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key still works: %d", rec.Code)
	}

	// Rotation: a new builtin value revokes the old one.
	if err := keySvc.EnsureBuiltin(ctx, "rsk_builtin_rotated"); err != nil {
		t.Fatalf("rotate builtin: %v", err)
	}
	if rec := anon("/api/public/v1/schedules", "rsk_builtin_test_value"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old builtin value still works: %d", rec.Code)
	}
	if rec := anon("/api/public/v1/schedules", "rsk_builtin_rotated"); rec.Code != http.StatusOK {
		t.Fatalf("rotated builtin value: %d", rec.Code)
	}

	// With the schedule switched off, the whole surface stays 404 (the
	// switch runs before key auth, so nothing leaks).
	cfg.PublicScheduleEnabled = false
	offRouter := rallyhttp.NewRouter(rallyhttp.Deps{
		Config: cfg,
		Pool:   pool,
		Logger: zerolog.Nop(),
		Brand:  brand.New(cfg),
		Access: auth.DefaultAccessConfig(),
		Keys:   keySvc,
	}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/public/v1/schedules", nil)
	req.Header.Set("Authorization", "Bearer rsk_builtin_rotated")
	offRec := httptest.NewRecorder()
	offRouter.ServeHTTP(offRec, req)
	if offRec.Code != http.StatusNotFound {
		t.Fatalf("switched off: %d", offRec.Code)
	}
}
