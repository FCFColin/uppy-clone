package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/resilience"
)

func TestConfigRepository_GetConfig_Found(t *testing.T) {
	repo, mock := newMockConfigRepository(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "config", "updated_at"}).
		AddRow("global", `{"x":1}`, int64(100))
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(rows)

	cfg, err := repo.GetConfig(ctx, "global")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg == nil || cfg.ID != "global" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestConfigRepository_GetConfig_NotFound(t *testing.T) {
	repo, mock := newMockConfigRepository(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("missing").
		WillReturnError(pgx.ErrNoRows)

	cfg, err := repo.GetConfig(ctx, "missing")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil, got %+v", cfg)
	}
}

func TestConfigRepository_GetConfig_Error(t *testing.T) {
	repo, mock := newMockConfigRepository(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("err").
		WillReturnError(fmt.Errorf("query failed"))

	_, err := repo.GetConfig(ctx, "err")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConfigRepository_SaveConfig_Success(t *testing.T) {
	repo, mock := newMockConfigRepository(t)
	ctx := context.Background()

	cfg := &domain.AppConfig{ID: "global", Config: `{"x":1}`, UpdatedAt: 100}

	mock.ExpectExec("INSERT INTO admin_config").
		WithArgs(cfg.ID, cfg.Config, cfg.UpdatedAt).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))

	if err := repo.SaveConfig(ctx, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
}

func TestConfigRepository_SaveConfig_Error(t *testing.T) {
	repo, mock := newMockConfigRepository(t)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO admin_config").
		WillReturnError(errors.New("save failed"))

	err := repo.SaveConfig(ctx, &domain.AppConfig{ID: "global", Config: `{}`})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresStore_ObservePoolStats_NilPool(t *testing.T) {
	db := &PostgresStore{cb: resilience.NewPostgresBreaker()}
	db.ObservePoolStats() // no-op when pool is not *pgxpool.Pool
}

func TestPostgresStore_ObservePoolStats_RealPool(t *testing.T) {
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	timeouts := config.DefaultTimeoutConfig()
	db, err := NewPostgresStore(connStr, timeouts)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	t.Cleanup(db.Close)

	db.ObservePoolStats()
	stat := db.PoolStats()
	if stat == nil {
		t.Fatal("expected pool stats")
	}
	// Call a second time to test delta calculation.
	db.ObservePoolStats()
}

func TestPostgresStore_ObservePoolStats_AcquireDelta(t *testing.T) {
	var db PostgresStore
	db.cb = resilience.NewPostgresBreaker()

	// Use a real connection if available.
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	timeouts := config.DefaultTimeoutConfig()
	realDB, err := NewPostgresStore(connStr, timeouts)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	t.Cleanup(realDB.Close)

	realDB.ObservePoolStats()
	time.Sleep(10 * time.Millisecond)
	realDB.ObservePoolStats()
}