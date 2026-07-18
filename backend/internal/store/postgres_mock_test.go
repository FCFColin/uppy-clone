package store

import (
	"testing"

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
func newMockRepo[T any](t *testing.T, newFn func(pgPool, ...Deps) T) (T, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return newFn(mock), mock
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
