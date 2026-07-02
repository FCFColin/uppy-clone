package store

import (
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

func newMockPostgresStore(t *testing.T) (*PostgresStore, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return NewPostgresStoreWithPool(mock), mock
}

func TestNewPostgresStoreWithPool(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	s := NewPostgresStoreWithPool(mock)
	if s == nil || s.PoolStats() != nil {
		t.Fatalf("PoolStats on mock pool should be nil, got %v", s.PoolStats())
	}
	s.Close()
}
