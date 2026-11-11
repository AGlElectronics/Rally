package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/AGlElectronics/rally/backend/internal/brand"
	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/denlink"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// newTestRouter builds the real router without a database; only routes that
// answer before touching the pool can be exercised.
func newTestRouter(t *testing.T, cfg *config.Config) http.Handler {
	t.Helper()
	den := denlink.New(nil, denlink.Config{Slug: "test", AppEnv: "development"}, zerolog.Nop())
	return NewRouter(Deps{Config: cfg, Logger: zerolog.Nop(), Den: den, Brand: brand.New(cfg)}, "test")
}

func TestAPIResponsesCarrySecurityHeaders(t *testing.T) {
	router := newTestRouter(t, &config.Config{AuthMode: config.AuthModeOIDC})
	for _, path := range []string{"/api/den/state", "/api/v1/editions", "/api/does-not-exist"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		for k, want := range map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"X-Frame-Options":         "DENY",
			"Referrer-Policy":         "no-referrer",
			"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		} {
			if got := rec.Header().Get(k); got != want {
				t.Errorf("GET %s: %s = %q, want %q", path, k, got, want)
			}
		}
	}
}

func preflightHeaders(t *testing.T, cfg *config.Config) string {
	t.Helper()
	cfg.CORSAllowedOrigins = []string{"https://events.example.org"}
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/me", nil)
	req.Header.Set("Origin", "https://events.example.org")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization, x-dev-user")
	rec := httptest.NewRecorder()
	newTestRouter(t, cfg).ServeHTTP(rec, req)
	return strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORSAllowsDevHeadersOnlyInDevMode(t *testing.T) {
	if got := preflightHeaders(t, &config.Config{AuthMode: config.AuthModeOIDC, AppEnv: "production"}); strings.Contains(got, "x-dev-user") {
		t.Errorf("oidc mode allows %q", got)
	}
	// AUTH_MODE=dev outside development is refused at start-up; should a
	// router still be built with it, it must not trust the dev headers.
	if got := preflightHeaders(t, &config.Config{AuthMode: config.AuthModeDev, AppEnv: "production"}); strings.Contains(got, "x-dev-user") {
		t.Errorf("dev mode outside development allows %q", got)
	}
	if got := preflightHeaders(t, &config.Config{AuthMode: config.AuthModeDev, AppEnv: "development"}); !strings.Contains(got, "x-dev-user") {
		t.Errorf("dev mode in development: Access-Control-Allow-Headers = %q, want X-Dev-User", got)
	}
}

func TestPublicRateLimit(t *testing.T) {
	router := newTestRouter(t, &config.Config{AuthMode: config.AuthModeOIDC, PublicRateLimit: config.RateLimit{Rate: 0.001, Burst: 2}})
	get := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/public/v1/brand", nil)
		req.RemoteAddr = ip + ":40000"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for i := 0; i < 2; i++ {
		if rec := get("203.0.113.7"); rec.Code != http.StatusOK {
			t.Fatalf("request %d = %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec := get("203.0.113.7")
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusTooManyRequests || body.Error.Code != "rate_limited" || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("third request = %d %s (Retry-After %q), want 429 rate_limited", rec.Code, rec.Body.String(), rec.Header().Get("Retry-After"))
	}
	if rec := get("198.51.100.9"); rec.Code != http.StatusOK {
		t.Fatalf("another client = %d, want its own bucket", rec.Code)
	}
	// Staff routes are not covered by the public limiter.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/brand", nil)
	req.RemoteAddr = "203.0.113.7:40000"
	staff := httptest.NewRecorder()
	router.ServeHTTP(staff, req)
	if staff.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/brand = %d, want 200", staff.Code)
	}
}

func TestDecodeJSONLimitsAndErrors(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	decode := func(body string, optional bool) (in, error) {
		var v in
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if optional {
			return v, decodeOptionalJSON(httptest.NewRecorder(), req, &v)
		}
		return v, decodeJSON(httptest.NewRecorder(), req, &v)
	}
	code := func(err error) httperror.Code {
		var he *httperror.Error
		if !errors.As(err, &he) {
			return ""
		}
		return he.Code
	}

	if v, err := decode(`{"name":"Ada"}`, false); err != nil || v.Name != "Ada" {
		t.Fatalf("valid body: %+v %v", v, err)
	}
	if _, err := decode(`{"nam":"Ada"}`, false); code(err) != httperror.CodeBadRequest {
		t.Errorf("unknown field: %v", err)
	}
	if _, err := decode(``, false); code(err) != httperror.CodeBadRequest {
		t.Errorf("empty body: %v", err)
	}
	if _, err := decode(``, true); err != nil {
		t.Errorf("empty optional body: %v", err)
	}
	if _, err := decode(`{"name":`, true); code(err) != httperror.CodeBadRequest {
		t.Errorf("broken optional body: %v", err)
	}
	big := `{"name":"` + string(bytes.Repeat([]byte("a"), int(maxJSONBody))) + `"}`
	if _, err := decode(big, false); code(err) != httperror.CodeRequestTooLarge {
		t.Errorf("body over the limit: %v, want request_too_large", err)
	}
	rec := httptest.NewRecorder()
	httperror.Render(rec, httptest.NewRequest(http.MethodPost, "/", nil), httperror.New(httperror.CodeRequestTooLarge, "too big"))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("request_too_large renders %d, want 413", rec.Code)
	}
}
