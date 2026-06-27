package middleware

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

func setupMiniredisStore(t *testing.T) *store.RedisStore {
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
