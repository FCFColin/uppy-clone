// Package testutil provides shared test helpers for backend tests.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// SetupRedisClient starts Redis via testcontainers and returns a connected go-redis client.
func SetupRedisClient(t *testing.T) (*redis.Client, context.Context) {
	t.Helper()
	ctx, addr := startRedisContainer(t)

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	return rdb, ctx
}

// SetupMiniredisStore returns a RedisStore backed by miniredis for fast unit tests.
func SetupMiniredisStore(t *testing.T) *store.RedisStore {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	timeouts := config.TimeoutConfig{
		RedisConnectTimeout: time.Second,
		RedisReadTimeout:    time.Second,
		RedisWriteTimeout:   time.Second,
	}
	rdb, err := store.NewRedisStore(mr.Addr(), timeouts)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

// NewTestMiniredis starts a miniredis instance and returns it with a connected
// go-redis client. Lightweight alternative to SetupRedisClient for unit tests
// that need a raw *redis.Client (not a full *store.RedisStore).
func NewTestMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, rdb
}
