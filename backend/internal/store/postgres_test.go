package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestPostgresStore_NewRequiresDatabaseURL(t *testing.T) {
	_, err := NewPostgresStore("", config.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected error for empty database URL")
	}
}

func TestPostgresStore_NewInvalidConnString(t *testing.T) {
	t.Parallel()
	_, err := NewPostgresStore("://not-a-valid-dsn", config.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected error for invalid connection string")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("expected parse config error, got %v", err)
	}
}

// mockRows implements pgx.Rows for testing scanLobbyRows.
type mockRows struct {
	data    []domain.LobbyState
	pos     int
	closed  bool
	err     error
	scanErr error
}

func (m *mockRows) Close()                                       { m.closed = true }
func (m *mockRows) Err() error                                   { return m.err }
func (m *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (m *mockRows) Conn() *pgx.Conn                              { return nil }
func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockRows) Next() bool {
	if m.err != nil || m.pos >= len(m.data) {
		return false
	}
	m.pos++
	return m.pos <= len(m.data)
}
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
func (m *mockRows) RawValues() [][]byte    { return nil }
func (m *mockRows) Values() ([]any, error) { return nil, nil }

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
			data:    []domain.LobbyState{{ID: "id1"}},
			scanErr: errors.New("scan failed"),
		}
		_, err := scanLobbyRows(rows)
		if err == nil || !strings.Contains(err.Error(), "scan failed") {
			t.Errorf("expected scan error, got %v", err)
		}
	})

	t.Run("rows.Err propagates", func(t *testing.T) {
		rows := &mockRows{
			data: []domain.LobbyState{{ID: "id1"}},
			err:  errors.New("iteration error"),
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
