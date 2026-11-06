// Package config loads and validates the backend runtime configuration from
// environment variables (12-factor). Validation runs once at startup and
// fails loudly: a misconfigured deployment is better than a silently
// degraded one.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// AuthMode selects how incoming requests are authenticated.
//
//   - AuthModeDev reads an X-Dev-User header and provisions a local user
//     tied to the "dev" issuer. Only safe for local scaffolding and CI.
//   - AuthModeOIDC verifies a Bearer JWT via the configured issuer.
type AuthMode string

const (
	AuthModeOIDC AuthMode = "oidc"
	AuthModeDev  AuthMode = "dev"
)

// Config is the validated process configuration. Fields are grouped by
// concern so new flags can be added next to related ones.
type Config struct {
	// Core
	AppEnv     string
	AppBaseURL string
	HTTPAddr   string

	// Database
	DatabaseURL string

	// Object storage (MinIO in local dev ONLY — see docs/adr/0004).
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

	// Attendee login for the public submission site: a separate realm from
	// staff. Empty issuer in oidc mode = public sign-in is unavailable.
	AttendeeOIDCIssuerURL string
	AttendeeOIDCClientID  string

	// Brand provider
	BrandLocalPath string // optional brand.json; Den's brand overlays it once paired
	// DenInternalURL replaces the host of pasted Den pairing URLs (Docker).
	DenInternalURL string

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
	ShutdownTimeout    time.Duration
}

// Load reads environment variables (plus an optional .env file) and returns
// a validated Config. Required fields that are missing return an aggregated
// error so operators see every problem at once instead of fixing one at a
// time.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv: getEnv("APP_ENV", "development"),
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

		AuthMode:         AuthMode(getEnv("AUTH_MODE", "dev")),
		OIDCIssuerURL:    getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:     getEnv("OIDC_CLIENT_ID", ""),
		OIDCAudience:     getEnv("OIDC_AUDIENCE", ""),
		AccessConfigPath: getEnv("ACCESS_CONFIG_PATH", ""),
		DevDefaultUser:   getEnv("DEV_DEFAULT_USER", "dev-admin"),

		AttendeeOIDCIssuerURL: getEnv("ATTENDEE_OIDC_ISSUER_URL", ""),
		AttendeeOIDCClientID:  getEnv("ATTENDEE_OIDC_CLIENT_ID", ""),

		BrandLocalPath: getEnv("BRAND_LOCAL_PATH", ""),
		DenInternalURL: getEnv("DEN_INTERNAL_URL", ""),

		PublicScheduleEnabled: getEnvBool("PUBLIC_SCHEDULE_ENABLED", false),
		PublicAPIBuiltinKey:   getEnv("PUBLIC_API_BUILTIN_KEY", ""),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramAPIURL:        getEnv("TELEGRAM_API_URL", "https://api.telegram.org"),

		CORSAllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3002")),
		ShutdownTimeout:    getEnvDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
	}

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
	default:
		errs = append(errs, fmt.Sprintf("AUTH_MODE must be 'oidc' or 'dev', got %q", c.AuthMode))
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
