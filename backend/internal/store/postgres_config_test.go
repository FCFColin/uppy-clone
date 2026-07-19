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
)

func TestConfigRepository_GetConfig(t *testing.T) {
	tests := []struct {
		name     string
		queryErr error
		rows     *pgxmock.Rows
		wantNil  bool
		wantErr  bool
	}{
		{
			name: "found",
			rows: pgxmock.NewRows([]string{"id", "config", "updated_at"}).
				AddRow("global", `{"x":1}`, int64(100)),
		},
		{
			name:     "not found",
			queryErr: pgx.ErrNoRows,
			wantNil:  true,
		},
		{
			name:     "error",
			queryErr: fmt.Errorf("query failed"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewConfigRepository)
			q := mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
				WithArgs(pgxmock.AnyArg())
			if tt.queryErr != nil {
				q.WillReturnError(tt.queryErr)
			} else {
				q.WillReturnRows(tt.rows)
			}
			cfg, err := repo.GetConfig(context.Background(), "global")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetConfig: %v", err)
			}
			if tt.wantNil && cfg != nil {
				t.Fatalf("expected nil, got %+v", cfg)
			}
			if !tt.wantNil && cfg == nil {
				t.Fatal("expected non-nil config")
			}
		})
	}
}

func TestConfigRepository_SaveConfig(t *testing.T) {
	tests := []struct {
		name    string
		execErr error
		wantErr bool
	}{
		{"success", nil, false},
		{"error", errors.New("save failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewConfigRepository)
			cfg := &domain.AppConfig{ID: "global", Config: `{"x":1}`, UpdatedAt: 100}
			exec := mock.ExpectExec("INSERT INTO admin_config").
				WithArgs(cfg.ID, cfg.Config, cfg.UpdatedAt)
			if tt.execErr != nil {
				exec.WillReturnError(tt.execErr)
			} else {
				exec.WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
			}
			err := repo.SaveConfig(context.Background(), cfg)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("SaveConfig: %v", err)
			}
		})
	}
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
	db.cb = DefaultDeps().PostgresBreakerFactory()

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
