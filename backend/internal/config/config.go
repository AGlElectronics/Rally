// Package config loads and validates the backend runtime configuration from
// environment variables (12-factor). Validation runs once at startup and
// fails loudly: a misconfigured deployment is better than a silently
// degraded one.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/AGlElectronics/rally/backend/internal/authbase"
	"github.com/AGlElectronics/rally/backend/internal/clientip"
	"github.com/AGlElectronics/rally/backend/internal/denlink"
	"github.com/AGlElectronics/rally/backend/internal/groupmatch"
)

// AuthMode selects how incoming requests are authenticated (AUTH_MODE,
// read and guarded by the shared authbase package).
//
//   - AuthModeDev reads an X-Dev-User header and provisions a local user
//     tied to the "dev" issuer. Only allowed with APP_ENV=development.
//   - AuthModeOIDC (the default) verifies a Bearer JWT via the configured issuer.
type AuthMode = authbase.AuthMode

const (
	AuthModeOIDC = authbase.AuthOIDC
	AuthModeDev  = authbase.AuthDev
)

// Default rate limits (authbase.ParseRate syntax) and bursts.
const (
	DefaultPublicRateLimit     = "120/m"
	DefaultPublicRateBurst     = 30
	DefaultSubmissionRateLimit = "20/m"
	DefaultSubmissionRateBurst = 10
)

// RateLimit is one limiter's setting. A zero Rate means off.
type RateLimit struct {
	Rate  float64 // requests per second
	Burst int
}

// DenSlug is the slug Rally pairs with Den under (Den's built-in name for it).
const DenSlug = "events"

// Config is the validated process configuration. Fields are grouped by
// concern so new flags can be added next to related ones.
type Config struct {
	// Core
	AppEnv     string
	AppBaseURL string
	HTTPAddr   string

	// Database
	DatabaseURL string

	// Object storage (SeaweedFS in local dev ONLY — see docs/adr/0004).
	S3Endpoint        string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	S3UsePathStyle    bool
	S3Enabled         bool // false = skip file storage init (useful for CI smoke)

	// Auth
	AuthMode         AuthMode
	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCAudience     string
	AccessConfigPath string
	DevDefaultUser   string
	// Groups is how staff group claims are read and compared with the
	// access config bindings (OIDC_GROUPS_CLAIM, OIDC_GROUP_MATCH,
	// OIDC_GROUP_PREFIX). Attendee tokens never use it.
	Groups groupmatch.Config

	// Attendee login for the public submission site: a separate realm from
	// staff. Empty issuer in oidc mode = public sign-in is unavailable.
	AttendeeOIDCIssuerURL string
	AttendeeOIDCClientID  string

	// Brand provider
	BrandLocalPath string // optional brand.json; Den's brand overlays it once paired
	// Den is the pairing configuration (DEN_MODE, DEN_INTERNAL_URL,
	// DEN_ALLOWED_HOSTS, DEN_LINK_ENCRYPTION_KEY[_PREVIOUS], APP_ENV).
	// Version is filled in by the app at start-up.
	Den denlink.Config

	// Public schedule API. Den's "events_public_schedule" setting wins when
	// paired and set; this is the standalone default.
	PublicScheduleEnabled bool
	// PublicAPIBuiltinKey is the "programme-page" API key the submission
	// site uses for the public schedule (set the same value as the
	// frontend's VITE_PUBLIC_SCHEDULE_KEY). It ships to every browser, so
	// it buys rotation and per-client tracking, not secrecy. Changing it
	// rotates the key; empty removes it.
	PublicAPIBuiltinKey string

	// Telegram bot for live-day announcements. Empty token = dry run: sends
	// are recorded but nothing leaves the server.
	TelegramBotToken string
	TelegramAPIURL   string

	// HTTP
	CORSAllowedOrigins []string
	// TrustedProxyCIDRs lists the proxies whose X-Forwarded-For entries are
	// believed when working out the client IP (TRUSTED_PROXY_CIDRS).
	TrustedProxyCIDRs []netip.Prefix
	ShutdownTimeout   time.Duration

	// PublicRateLimit applies per client IP to everything under
	// /api/public/v1 (PUBLIC_RATE_LIMIT, PUBLIC_RATE_BURST).
	PublicRateLimit RateLimit
	// SubmissionRateLimit applies per person (else per client IP) to the
	// endpoints that create submissions, invitations and uploads
	// (SUBMISSION_RATE_LIMIT, SUBMISSION_RATE_BURST).
	SubmissionRateLimit RateLimit
}

// Auth is the shared authentication guard for this configuration.
func (c *Config) Auth() authbase.Config {
	return authbase.Config{AppEnv: c.AppEnv, AuthMode: c.AuthMode}
}

// DevAuth reports AUTH_MODE=dev in a valid (APP_ENV=development) setup.
func (c *Config) DevAuth() bool { return c.Auth().DevAuth() }

// Load reads environment variables (plus an optional .env file) and returns
// a validated Config. Required fields that are missing return an aggregated
// error so operators see every problem at once instead of fixing one at a
// time.
func Load() (*Config, error) {
	_ = godotenv.Load()

	// AUTH_MODE (default oidc) and APP_ENV (default production); the guard
	// itself runs in validate so every problem is reported at once.
	auth, _ := authbase.FromEnv(os.Getenv)

	cfg := &Config{
		AppEnv: auth.AppEnv,
		// Container-internal defaults: every module in the ecosystem binds
		// :8080 inside its container. Host-side port mapping (host 8100 ->
		// container 8080 for Rally) is handled by docker-compose / K8s.
		AppBaseURL: getEnv("APP_BASE_URL", "http://localhost:8080"),
		HTTPAddr:   getEnv("HTTP_ADDR", ":8080"),

		DatabaseURL: getEnv("DATABASE_URL", ""),

		S3Endpoint:        getEnv("S3_ENDPOINT", ""),
		S3Bucket:          getEnv("S3_BUCKET", ""),
		S3AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", ""),
		S3SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", ""),
		S3Region:          getEnv("S3_REGION", "us-east-1"),
		S3UsePathStyle:    getEnvBool("S3_USE_PATH_STYLE", true),
		S3Enabled:         getEnvBool("S3_ENABLED", true),

		AuthMode:         auth.AuthMode,
		OIDCIssuerURL:    getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:     getEnv("OIDC_CLIENT_ID", ""),
		OIDCAudience:     getEnv("OIDC_AUDIENCE", ""),
		AccessConfigPath: getEnv("ACCESS_CONFIG_PATH", ""),
		DevDefaultUser:   getEnv("DEV_DEFAULT_USER", "dev-admin"),

		AttendeeOIDCIssuerURL: getEnv("ATTENDEE_OIDC_ISSUER_URL", ""),
		AttendeeOIDCClientID:  getEnv("ATTENDEE_OIDC_CLIENT_ID", ""),

		BrandLocalPath: getEnv("BRAND_LOCAL_PATH", ""),

		PublicScheduleEnabled: getEnvBool("PUBLIC_SCHEDULE_ENABLED", false),
		PublicAPIBuiltinKey:   getEnv("PUBLIC_API_BUILTIN_KEY", ""),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramAPIURL:        getEnv("TELEGRAM_API_URL", "https://api.telegram.org"),

		CORSAllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3002")),
		ShutdownTimeout:    getEnvDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
	}

	trusted, err := clientip.ParseCIDRs(getEnv("TRUSTED_PROXY_CIDRS", clientip.DefaultTrustedProxies))
	if err != nil {
		return nil, fmt.Errorf("invalid config: TRUSTED_PROXY_CIDRS: %w", err)
	}
	cfg.TrustedProxyCIDRs = trusted

	if cfg.PublicRateLimit, err = rateLimit("PUBLIC_RATE_LIMIT", DefaultPublicRateLimit, "PUBLIC_RATE_BURST", DefaultPublicRateBurst); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if cfg.SubmissionRateLimit, err = rateLimit("SUBMISSION_RATE_LIMIT", DefaultSubmissionRateLimit, "SUBMISSION_RATE_BURST", DefaultSubmissionRateBurst); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Den's settings are validated together with the rest below, against the
	// same APP_ENV as everything else.
	cfg.Den, _ = denlink.FromEnv(DenSlug, "", os.Getenv)
	cfg.Den.AppEnv = cfg.AppEnv
	cfg.Groups, _ = groupmatch.FromEnv(os.Getenv)

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	var errs []string

	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if err := c.Den.Validate(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := c.Groups.Validate(); err != nil {
		errs = append(errs, err.Error())
	}

	if c.S3Enabled {
		if c.S3Endpoint == "" {
			errs = append(errs, "S3_ENDPOINT is required when S3_ENABLED=true")
		}
		if c.S3Bucket == "" {
			errs = append(errs, "S3_BUCKET is required when S3_ENABLED=true")
		}
		if c.S3AccessKeyID == "" {
			errs = append(errs, "S3_ACCESS_KEY_ID is required when S3_ENABLED=true")
		}
		if c.S3SecretAccessKey == "" {
			errs = append(errs, "S3_SECRET_ACCESS_KEY is required when S3_ENABLED=true")
		}
	}

	if err := c.Auth().Validate(); err != nil {
		errs = append(errs, err.Error())
	}

	switch c.AuthMode {
	case AuthModeOIDC:
		if c.OIDCIssuerURL == "" {
			errs = append(errs, "OIDC_ISSUER_URL is required when AUTH_MODE=oidc")
		}
		if c.OIDCClientID == "" {
			errs = append(errs, "OIDC_CLIENT_ID is required when AUTH_MODE=oidc")
		}
		if c.AttendeeOIDCIssuerURL != "" && c.AttendeeOIDCClientID == "" {
			errs = append(errs, "ATTENDEE_OIDC_CLIENT_ID is required when ATTENDEE_OIDC_ISSUER_URL is set")
		}
		if c.AttendeeOIDCIssuerURL != "" && c.AttendeeOIDCIssuerURL == c.OIDCIssuerURL {
			errs = append(errs, "ATTENDEE_OIDC_ISSUER_URL must be a different realm than OIDC_ISSUER_URL (staff)")
		}
	case AuthModeDev:
		// dev mode: no extra required fields; dev users auto-provision.
	}

	if len(errs) > 0 {
		return errors.New("invalid config: " + strings.Join(errs, "; "))
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// rateLimit reads a rate ("60/m", or "off") and its burst.
func rateLimit(rateKey, rateDefault, burstKey string, burstDefault int) (RateLimit, error) {
	raw := getEnv(rateKey, rateDefault)
	if strings.EqualFold(strings.TrimSpace(raw), "off") {
		return RateLimit{}, nil
	}
	rate, err := authbase.ParseRate(raw)
	if err != nil {
		return RateLimit{}, fmt.Errorf("%s: %w (or off)", rateKey, err)
	}
	burst := burstDefault
	if v := getEnv(burstKey, ""); v != "" {
		if burst, err = strconv.Atoi(strings.TrimSpace(v)); err != nil || burst < 1 {
			return RateLimit{}, fmt.Errorf("%s must be a whole number of at least 1, got %q", burstKey, v)
		}
	}
	return RateLimit{Rate: rate, Burst: burst}, nil
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
