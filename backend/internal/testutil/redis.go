package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// SetupRedisStore starts Redis via testcontainers and returns a connected RedisStore.
func SetupRedisStore(t *testing.T) (context.Context, *store.RedisStore) {
	t.Helper()
	skipIfShort(t)

	ctx := context.Background()
	redisContainer, err := tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(10*time.Second)),
	)
	if err != nil {
		t.Skipf("redis container unavailable: %v", err)
	}
	t.Cleanup(func() { _ = redisContainer.Terminate(ctx) })

	addr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("redis endpoint: %v", err)
	}

	timeouts := config.TimeoutConfig{
		RedisConnectTimeout: 5 * time.Second,
		RedisReadTimeout:    3 * time.Second,
		RedisWriteTimeout:   3 * time.Second,
	}

	rdb, err := store.NewRedisStore(addr, timeouts)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	return ctx, rdb
}
