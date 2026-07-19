package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestPostgresStore_NewInvalidInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		connStr string
		wantErr string // empty: only require err != nil
	}{
		{"empty database URL", "", ""},
		{"invalid connection string", "://not-a-valid-dsn", "parse config"},
		{"unreachable ping", "postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable&connect_timeout=1", "ping"},
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

func TestPostgresStore_NewPoolConfigError(t *testing.T) {
	if err := os.Setenv("PG_POOL_MAX_CONNS", "1"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("PG_POOL_MIN_CONNS", "5"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("PG_POOL_MAX_CONNS")
		_ = os.Unsetenv("PG_POOL_MIN_CONNS")
	})

	_, err := NewPostgresStore(
		"postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable&connect_timeout=1",
		config.DefaultTimeoutConfig(),
	)
	if err == nil {
		t.Fatal("expected pool config error")
	}
}

func TestPostgresStore_NewPostgresStore_Success(t *testing.T) {
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://test:test@127.0.0.1:5432/testdb?sslmode=disable&connect_timeout=2"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres not available: %v", err)
	}
	pool.Close()

	db, err := NewPostgresStore(connStr, config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(db.Close)
	if db.Pool() == nil || db.PoolStats() == nil {
		t.Fatal("expected live pool stats")
	}
}

func TestPostgresStore_NewPostgresStore_MockPool(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	mock.ExpectPing().WillReturnError(nil)

	var capturedCfg *pgxpool.Config
	orig := pgxNewWithConfigFn
	pgxNewWithConfigFn = func(_ context.Context, cfg *pgxpool.Config) (pgPool, error) {
		capturedCfg = cfg
		return mock, nil
	}
	t.Cleanup(func() { pgxNewWithConfigFn = orig })

	db, err := NewPostgresStore(
		"postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
		config.DefaultTimeoutConfig(),
	)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(db.Close)
	if capturedCfg == nil || capturedCfg.PrepareConn == nil {
		t.Fatal("expected PrepareConn on pool config")
	}
	ok, prepErr := capturedCfg.PrepareConn(context.Background(), nil)
	if prepErr != nil || !ok {
		t.Fatalf("PrepareConn: ok=%v err=%v", ok, prepErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStore_NewPostgresStore_CreatePoolError(t *testing.T) {
	orig := pgxNewWithConfigFn
	pgxNewWithConfigFn = func(_ context.Context, _ *pgxpool.Config) (pgPool, error) {
		return nil, errors.New("create failed")
	}
	t.Cleanup(func() { pgxNewWithConfigFn = orig })

	_, err := NewPostgresStore(
		"postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
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

func TestPgxNewWithConfigFn_DefaultSuccess(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	pool, err := pgxNewWithConfigFn(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pgxNewWithConfigFn: %v", err)
	}
	pool.Close()
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

// BenchmarkPostgresStore_ConcurrentLoad verifies pool behavior under concurrent load.
func BenchmarkPostgresStore_ConcurrentLoad(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping integration benchmark in short mode")
	}
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		b.Skip("TEST_DATABASE_URL not set, skipping")
	}

	timeouts := config.DefaultTimeoutConfig()
	db, err := NewPostgresStore(dbURL, timeouts)
	if err != nil {
		b.Fatalf("failed to connect: %v", err)
	}
	defer db.Close()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = db.Pool().Ping(context.Background())
		}
	})

	stat := db.PoolStats()
	if stat.AcquiredConns() > 0 {
		b.Logf("pool still has %d acquired conns after benchmark", stat.AcquiredConns())
	}
}
