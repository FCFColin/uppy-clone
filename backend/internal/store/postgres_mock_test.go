package store

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
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

func TestNewPostgresStoreWithPool(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	s := NewPostgresStoreWithPool(mock)
	if s == nil || s.PoolStats() != nil {
		t.Fatalf("PoolStats on mock pool should be nil, got %v", s.PoolStats())
	}
	s.Close()
}
