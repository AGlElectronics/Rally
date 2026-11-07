// Package app wires configuration, the database pool, object storage, auth
// and HTTP into a runnable server. cmd/server/main.go just calls Run.
//
// Deliberate omission vs the Stash reference: this package does NOT run
// Goose migrations on start. Migrations are a one-shot controlled operation
// handled by cmd/migrate (Compose service / K8s Job). See
// docs/adr/0003-migrate-job.md.
package app

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/brand"
	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/denlink"
	"github.com/AGlElectronics/rally/backend/internal/events"
	"github.com/AGlElectronics/rally/backend/internal/files"
	httpx "github.com/AGlElectronics/rally/backend/internal/http"
	"github.com/AGlElectronics/rally/backend/internal/tlscert"
)

// Version is set via -ldflags at build time; left as "dev" for local runs.
var Version = "dev"

// Run boots the server and blocks until a shutdown signal is received or
// ListenAndServe fails.
func Run(ctx context.Context, logger zerolog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger.Info().
		Str("env", cfg.AppEnv).
		Str("addr", cfg.HTTPAddr).
		Str("auth_mode", string(cfg.AuthMode)).
		Str("version", Version).
		Msg("starting rally backend")

	// DB pool. Migrations are NOT applied here; cmd/migrate ran first.
	dbCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := db.Open(dbCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer pool.Close()
	logger.Info().Msg("database pool ready")

	// Object storage. Optional so CI / smoke runs don't need a dev S3 server or an
	// S3 endpoint. Production MUST configure a real managed bucket.
	var filesClient *files.Client
	if cfg.S3Enabled {
		filesCtx, cancelFiles := context.WithTimeout(ctx, 30*time.Second)
		filesClient, err = files.New(filesCtx, cfg)
		cancelFiles()
		if err != nil {
			return fmt.Errorf("init files: %w", err)
		}
		logger.Info().Str("bucket", filesClient.Bucket()).Msg("object storage ready")
	} else {
		logger.Warn().Msg("S3_ENABLED=false — object storage is disabled")
	}

	// OIDC verifier. Only required when AUTH_MODE=oidc; dev mode skips.
	var oidcVerifier *auth.OIDCVerifier
	if cfg.AuthMode == config.AuthModeOIDC {
		oidcCtx, cancelOIDC := context.WithTimeout(ctx, 30*time.Second)
		oidcVerifier, err = auth.NewOIDCVerifier(oidcCtx, cfg)
		cancelOIDC()
		if err != nil {
			return fmt.Errorf("init oidc verifier: %w", err)
		}
		logger.Info().Str("issuer", cfg.OIDCIssuerURL).Msg("oidc verifier ready")
	}

	// Attendee realm for the public submission site. Optional: without it
	// the site lists open calls but can't sign anyone in.
	var attendeeVerifier *auth.OIDCVerifier
	if cfg.AuthMode == config.AuthModeOIDC && cfg.AttendeeOIDCIssuerURL != "" {
		attCtx, cancelAtt := context.WithTimeout(ctx, 30*time.Second)
		attendeeVerifier, err = auth.NewOIDCVerifierFor(attCtx, cfg.AttendeeOIDCIssuerURL, cfg.AttendeeOIDCClientID)
		cancelAtt()
		if err != nil {
			return fmt.Errorf("init attendee oidc verifier: %w", err)
		}
		logger.Info().Str("issuer", cfg.AttendeeOIDCIssuerURL).Msg("attendee oidc verifier ready")
	} else if cfg.AuthMode == config.AuthModeOIDC {
		logger.Warn().Msg("ATTENDEE_OIDC_ISSUER_URL not set — public submission sign-in is disabled")
	}

	// Access config catalog. The scaffold ships a built-in default; the
	// TOML/JSON loader stub ensures a bogus ACCESS_CONFIG_PATH fails loudly.
	var access *auth.AccessConfig
	if cfg.AccessConfigPath != "" {
		access, err = auth.LoadAccessConfig(cfg.AccessConfigPath)
		if err != nil {
			return fmt.Errorf("load access config: %w", err)
		}
		logger.Info().
			Str("path", cfg.AccessConfigPath).
			Strs("roles", access.Roles()).
			Msg("access config loaded")
	} else {
		access = auth.DefaultAccessConfig()
		logger.Info().Msg("no access config file configured; using built-in role catalog")
	}

	brandProvider := brand.New(cfg)
	den := denlink.New(pool, denlink.Config{Slug: "events", Version: Version, InternalURL: cfg.DenInternalURL}, logger)
	go den.Run(ctx)

	fileSvc := events.NewFileService(pool, filesClient)
	go fileSvc.Run(ctx, logger)

	// The public schedule API needs a key; the builtin "programme-page"
	// one comes from config so the submission site can use it.
	keySvc := events.NewKeyService(pool)
	if err := keySvc.EnsureBuiltin(ctx, cfg.PublicAPIBuiltinKey); err != nil {
		return fmt.Errorf("ensure builtin public API key: %w", err)
	}

	router := httpx.NewRouter(httpx.Deps{
		Config:           cfg,
		Pool:             pool,
		Logger:           logger,
		Files:            filesClient,
		FileService:      fileSvc,
		Keys:             keySvc,
		Brand:            brandProvider,
		Den:              den,
		OIDCVerifier:     oidcVerifier,
		AttendeeVerifier: attendeeVerifier,
		Access:           access,
	}, Version)

	server := &nethttp.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if _, err := tlscert.Configure(ctx, server, logger); err != nil {
		return fmt.Errorf("tls: %w", err)
	}

	return runServer(ctx, server, pool, cfg.ShutdownTimeout, logger)
}

func runServer(ctx context.Context, server *nethttp.Server, pool *pgxpool.Pool, shutdownTimeout time.Duration, logger zerolog.Logger) error {
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", server.Addr).Msg("http listening")
		if err := tlscert.ListenAndServe(server); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-signalCtx.Done():
		logger.Info().Msg("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("http shutdown error")
	}
	pool.Close()
	logger.Info().Msg("shutdown complete")
	return nil
}
