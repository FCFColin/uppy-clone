package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/resilience"
)

func TestPostgresStore_GetConfig_Found(t *testing.T) {
	t.Parallel()
	db, mock := newMockPostgresStore(t)
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(pgxmock.NewRows([]string{"id", "config", "updated_at"}).
			AddRow("global", `{"email_enabled":true}`, int64(1000)))

	cfg, err := db.GetConfig(context.Background(), "global")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg == nil || cfg.ID != "global" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStore_GetConfig_NotFound(t *testing.T) {
	t.Parallel()
	db, mock := newMockPostgresStore(t)
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("missing").
		WillReturnError(pgx.ErrNoRows)

	cfg, err := db.GetConfig(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}
}

func TestPostgresStore_GetConfig_QueryError(t *testing.T) {
	t.Parallel()
	db, mock := newMockPostgresStore(t)
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnError(errors.New("db down"))

	cfg, err := db.GetConfig(context.Background(), "global")
	if err == nil {
		t.Fatal("expected error")
	}
	if cfg != nil {
		t.Fatal("expected nil config on error")
	}
}

func TestPostgresStore_SaveConfig(t *testing.T) {
	t.Parallel()
	db, mock := newMockPostgresStore(t)
	cfg := &domain.AppConfig{ID: "global", Config: `{"email_enabled":true}`, UpdatedAt: 2000}
	mock.ExpectExec(`INSERT INTO admin_config`).
		WithArgs(cfg.ID, cfg.Config, cfg.UpdatedAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := db.SaveConfig(context.Background(), cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStore_SaveConfig_Error(t *testing.T) {
	t.Parallel()
	db, mock := newMockPostgresStore(t)
	cfg := &domain.AppConfig{ID: "global", Config: `{}`, UpdatedAt: 1}
	mock.ExpectExec(`INSERT INTO admin_config`).
		WithArgs(cfg.ID, cfg.Config, cfg.UpdatedAt).
		WillReturnError(errors.New("write failed"))

	if err := db.SaveConfig(context.Background(), cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresStore_CloseAndPool(t *testing.T) {
	t.Parallel()
	db, mock := newMockPostgresStore(t)
	db.Close()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if db.Pool() != nil {
		t.Error("pgxmock pool should not cast to *pgxpool.Pool")
	}
	if db.PoolStats() != nil {
		t.Error("PoolStats should be nil for mock pool")
	}
}

func TestPostgresStore_ObservePoolStats_NilPool(t *testing.T) {
	t.Parallel()
	db, _ := newMockPostgresStore(t)
	db.ObservePoolStats() // no-op when pool is not *pgxpool.Pool
}

func TestPostgresStore_ObservePoolStats_RealPool(t *testing.T) {
	t.Parallel()
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	db := &PostgresStore{
		pool: pool,
		cb:   resilience.NewPostgresBreaker(),
	}
	db.ObservePoolStats()
	db.lastAcquireDuration.Store(1.0)
	db.lastAcquireCount.Store(int64(0))
	db.ObservePoolStats()
}

func TestSetRunMigrationsHook_Restore(t *testing.T) {
	restore := SetRunMigrationsHook(func(ctx context.Context, _, _ string) error {
		_ = ctx
		return nil
	})
	defer restore()
}

func TestPostgresStore_RunMigrations_RequiresRealPool(t *testing.T) {
	t.Parallel()
	db, _ := newMockPostgresStore(t)
	if err := db.RunMigrations("migrations"); err == nil {
		t.Fatal("expected error for mock pool")
	}
}

func TestPostgresStore_RunMigrations_SuccessHooked(t *testing.T) {
	t.Parallel()
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	orig := runMigrationsFn
	runMigrationsFn = func(_ context.Context, _, _ string) error { return nil }
	t.Cleanup(func() { runMigrationsFn = orig })

	db := NewPostgresStoreWithPool(pool)
	if err := db.RunMigrations("migrations"); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}

func TestPostgresStore_RunMigrations_ErrorHooked(t *testing.T) {
	t.Parallel()
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	orig := runMigrationsFn
	runMigrationsFn = func(_ context.Context, _, _ string) error { return fmt.Errorf("migrate failed") }
	t.Cleanup(func() { runMigrationsFn = orig })

	db := NewPostgresStoreWithPool(pool)
	if err := db.RunMigrations("migrations"); err == nil {
		t.Fatal("expected migration error")
	}
}

func TestPostgresStore_PoolStats_RealPool(t *testing.T) {
	t.Parallel()
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	db := NewPostgresStoreWithPool(pool)
	if stat := db.PoolStats(); stat == nil {
		t.Fatal("PoolStats should return stats for real pgxpool")
	}
}

func TestPostgresStore_ObservePoolStats_AcquireDelta(t *testing.T) {
	t.Parallel()
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	db := &PostgresStore{pool: pool, cb: resilience.NewPostgresBreaker()}
	db.ObservePoolStats()
	db.lastAcquireDuration.Store(0.0)
	db.lastAcquireCount.Store(int64(0))
	_ = pool.Ping(context.Background())
	db.ObservePoolStats()
}

func TestPostgresStore_RunMigrations_WithPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres")
	}
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://test:test@127.0.0.1:5432/testdb?sslmode=disable&connect_timeout=2"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := pgxpool.New(ctx, connStr); err != nil {
		t.Skipf("postgres not available: %v", err)
	}

	db, err := NewPostgresStore(connStr, config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(db.Close)

	if err := db.RunMigrations("migrations"); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}
