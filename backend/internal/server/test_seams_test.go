package server

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// newMockPool creates a pgxpool pointing at a non-existent postgres instance
// with a short connect timeout. Tests use it together with mocks that bypass
// actual network calls. The pool is closed via t.Cleanup.
func newMockPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// withMockPostgresStore replaces newPostgresStoreFn with a function that
// returns a PostgresStore backed by the given pool. The original function
// is restored via t.Cleanup.
func withMockPostgresStore(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	orig := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = orig })
}

// withMockRedisStore replaces newRedisStoreFn with a function that returns
// the given redisStore. The original function is restored via t.Cleanup.
func withMockRedisStore(t *testing.T, redisStore *store.RedisStore) {
	t.Helper()
	orig := newRedisStoreFn
	newRedisStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.RedisStore, error) {
		return redisStore, nil
	}
	t.Cleanup(func() { newRedisStoreFn = orig })
}

// withMigrationsHook installs the given migrations hook and registers
// cleanup to restore the original. Pass nil to install a no-op hook
// (migrations succeed silently).
func withMigrationsHook(t *testing.T, fn func(context.Context, string, string) error) {
	t.Helper()
	if fn == nil {
		fn = func(context.Context, string, string) error { return nil }
	}
	t.Cleanup(store.SetRunMigrationsHook(fn))
}
