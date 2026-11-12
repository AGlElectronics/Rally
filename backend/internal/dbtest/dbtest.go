// Package dbtest connects a test to RALLY_TEST_DATABASE_URL and applies
// Goose migrations once per process. Every database test calls Pool, so it
// works on an empty database and does not depend on another test having
// migrated first or left the schema behind.
//
// The database is shared by the tests in the process. A test may delete
// its own rows; it must not drop or alter the schema. CI runs packages one
// at a time (-p 1) because they share one database when the variable is set.
package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/migrations"
)

// migrationLockID is the advisory-lock key for Rally's migrations ("rly-migr").
// A second process waits, then finds nothing to apply, instead of racing
// Goose. The wait is bounded by lock_timeout so a stuck session fails the
// test instead of hanging it.
const migrationLockID int64 = 0x726c792d6d696772

var (
	migrateOnce sync.Once
	migrateErr  error
)

// Pool returns a pool on RALLY_TEST_DATABASE_URL after migrations have been
// applied. It skips the test when the variable is unset.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("RALLY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RALLY_TEST_DATABASE_URL not set")
	}
	// Do not call t.Fatal inside Do: Fatalf stops the goroutine without
	// returning, and a later test would wait on Once forever.
	migrateOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		migrateErr = migrate(ctx, dsn)
	})
	if migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}

	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func migrate(ctx context.Context, dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	lock, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("connect for the migration lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if _, err := lock.ExecContext(ctx, `SET lock_timeout = '30s'`); err != nil {
		return fmt.Errorf("set lock timeout: %w", err)
	}
	if _, err := lock.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("take the migration lock: %w", err)
	}
	defer func() {
		_, _ = lock.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationLockID)
	}()

	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("goose_db_version")
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
