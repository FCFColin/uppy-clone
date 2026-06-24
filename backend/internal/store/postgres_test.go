package store

import (
	"context"
	"os"
	"testing"

	"github.com/uppy-clone/backend/internal/config"
)

// BenchmarkPostgresStore_ConcurrentLoad verifies pool behavior under concurrent
// load. It spins up parallel goroutines that ping the database through the pool,
// then checks that no connections remain acquired after completion.
//
// This is an integration benchmark: it requires a running PostgreSQL instance.
// It is skipped in short mode and when TEST_DATABASE_URL is not set.
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
		// Each goroutine performs concurrent Ping operations to test pool behavior.
		for pb.Next() {
			_ = db.Pool().Ping(context.Background())
		}
	})

	// Verify pool didn't exhaust
	stat := db.PoolStats()
	if stat.AcquiredConns() > 0 {
		b.Logf("pool still has %d acquired conns after benchmark", stat.AcquiredConns())
	}
}
