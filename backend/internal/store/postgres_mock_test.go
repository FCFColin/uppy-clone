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

func newMockUserRepository(t *testing.T) (*UserRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return NewUserRepository(mock), mock
}

func newMockLobbyRepository(t *testing.T) (*LobbyRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return NewLobbyRepository(mock), mock
}

func newMockResultRepository(t *testing.T) (*ResultRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return NewResultRepository(mock), mock
}

func newMockConfigRepository(t *testing.T) (*ConfigRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return NewConfigRepository(mock), mock
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
