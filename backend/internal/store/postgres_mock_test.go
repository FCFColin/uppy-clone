package store

import (
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/resilience"
)

func newMockPostgresStore(t *testing.T) (*PostgresStore, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return &PostgresStore{
		pool: mock,
		cb:   resilience.NewPostgresBreaker(),
	}, mock
}
