package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// newMockRepo creates a pgxmock pool and wraps it with the provided constructor.
// The mock pool is registered with t.Cleanup for automatic close.
// This is the single mock factory for store tests — replaces the 5 former
// per-repository factory functions (RO-031).
//
// Testing-strategy boundary (RO-048): pgxmock backs UNIT tests (pure logic,
// no build tag). For SQL correctness / migration / constraint tests use
// testutil.SetupPostgres (testcontainers, `//go:build integration`). See the
// boundary doc at the top of internal/testutil/postgres.go.
//
// Note: cannot use testutil.NewPgxMock here because testutil imports store
// (via miniredis.go/postgres.go/redis.go), which would create an import cycle.
func newMockRepo[T any](t *testing.T, newFn func(pgPool, ...Deps) T) (T, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return newFn(mock), mock
}

// expectExecResult configures a mock ExpectExec to return either an error or
// a success result. Consolidates the repeated
// `if tt.execErr != nil { exec.WillReturnError(...) } else { exec.WillReturnResult(...) }`
// pattern across store unit tests (F-001).
func expectExecResult(exec *pgxmock.ExpectedExec, execErr error, successTag string) {
	if execErr != nil {
		exec.WillReturnError(execErr)
	} else {
		exec.WillReturnResult(pgconn.NewCommandTag(successTag))
	}
}

// assertWantErr checks the error result against the test's wantErr expectation.
// Consolidates the repeated
// `if tt.wantErr && err == nil { t.Fatal(...) }; if !tt.wantErr && err != nil { t.Fatalf(...) }`
// pattern across store unit tests (F-001).
func assertWantErr(t *testing.T, err error, wantErr bool, methodName string) {
	t.Helper()
	if wantErr && err == nil {
		t.Fatal("expected error")
	}
	if !wantErr && err != nil {
		t.Fatalf("%s: %v", methodName, err)
	}
}

func TestPostgresStore_NewInvalidInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		connStr string
		wantErr string // empty: only require err != nil
	}{
		{"empty database URL", "", ""},
		{"invalid connection string", "://not-a-valid-dsn", "parse config"},
		{"unreachable ping", "postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable&connect_timeout=1", "ping"}, // pragma: allowlist secret
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPostgresStore(tt.connStr, config.DefaultTimeoutConfig())
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q error, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestPostgresStore_NewPostgresStore_CreatePoolError(t *testing.T) {
	orig := pgxNewWithConfigFn
	pgxNewWithConfigFn = func(_ context.Context, _ *pgxpool.Config) (pgPool, error) {
		return nil, errors.New("create failed")
	}
	t.Cleanup(func() { pgxNewWithConfigFn = orig })

	_, err := NewPostgresStore(
		"postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable", // pragma: allowlist secret
		config.DefaultTimeoutConfig(),
	)
	if err == nil || !strings.Contains(err.Error(), "create pool") {
		t.Fatalf("expected create pool error, got %v", err)
	}
}

func TestPgxNewWithConfigFn_DefaultNilConfigError(t *testing.T) {
	_, err := pgxNewWithConfigFn(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "nil pool config") {
		t.Fatalf("expected nil pool config error, got %v", err)
	}
}

// mockRowsBase provides shared pgx.Rows methods for test mocks. Concrete mock
// row types embed this and implement Scan() + Next() with their own data shape.
type mockRowsBase struct {
	pos     int
	closed  bool
	err     error
	scanErr error
}

func (m *mockRowsBase) Close()                                       { m.closed = true }
func (m *mockRowsBase) Err() error                                   { return m.err }
func (m *mockRowsBase) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (m *mockRowsBase) Conn() *pgx.Conn                              { return nil }
func (m *mockRowsBase) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockRowsBase) RawValues() [][]byte                          { return nil }
func (m *mockRowsBase) Values() ([]any, error)                       { return nil, nil }

// next advances pos for a slice of length n. Returns false at end or on err.
func (m *mockRowsBase) next(n int) bool {
	if m.err != nil || m.pos >= n {
		return false
	}
	m.pos++
	return m.pos <= n
}

// mockRows implements pgx.Rows for testing scanLobbyRows.
type mockRows struct {
	mockRowsBase
	data []domain.LobbyState
}

func (m *mockRows) Next() bool { return m.next(len(m.data)) }
func (m *mockRows) Scan(dest ...interface{}) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	if m.pos == 0 || m.pos > len(m.data) {
		return errors.New("scan called out of range")
	}
	ls := m.data[m.pos-1]
	for i, d := range dest {
		switch i {
		case 0:
			*d.(*string) = ls.ID
		case 1:
			*d.(*string) = ls.Code
		case 2:
			*d.(*string) = ls.State
		case 3:
			*d.(*int64) = ls.UpdatedAt
		case 4:
			*d.(*int64) = ls.CreatedAt
		default:
			return errors.New("unexpected dest index")
		}
	}
	return nil
}

func TestScanLobbyRows(t *testing.T) {
	t.Parallel()

	t.Run("scans multiple rows", func(t *testing.T) {
		rows := &mockRows{
			data: []domain.LobbyState{
				{ID: "id1", Code: "A1", State: "waiting", UpdatedAt: 100, CreatedAt: 50},
				{ID: "id2", Code: "B2", State: "playing", UpdatedAt: 200, CreatedAt: 100},
			},
		}
		result, err := scanLobbyRows(rows)
		if err != nil {
			t.Fatalf("scanLobbyRows error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("got %d rows, want 2", len(result))
		}
		if result[0].Code != "A1" || result[1].Code != "B2" {
			t.Errorf("unexpected rows: %+v", result)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		rows := &mockRows{data: nil}
		result, err := scanLobbyRows(rows)
		if err != nil {
			t.Fatalf("scanLobbyRows error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty, got %d rows", len(result))
		}
	})

	t.Run("scan error propagates", func(t *testing.T) {
		rows := &mockRows{
			data:         []domain.LobbyState{{ID: "id1"}},
			mockRowsBase: mockRowsBase{scanErr: errors.New("scan failed")},
		}
		_, err := scanLobbyRows(rows)
		if err == nil || !strings.Contains(err.Error(), "scan failed") {
			t.Errorf("expected scan error, got %v", err)
		}
	})

	t.Run("rows.Err propagates", func(t *testing.T) {
		rows := &mockRows{
			data:         []domain.LobbyState{{ID: "id1"}},
			mockRowsBase: mockRowsBase{err: errors.New("iteration error")},
		}
		_, err := scanLobbyRows(rows)
		if err == nil || !strings.Contains(err.Error(), "iteration error") {
			t.Errorf("expected iteration error, got %v", err)
		}
	})
}

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


