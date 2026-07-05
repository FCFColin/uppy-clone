package store

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/resilience"
)

// RedisStore wraps a go-redis client with circuit breaker protection.
type RedisStore struct {
	rdb *redis.Client
	cb  *gobreaker.CircuitBreaker[any]
}

// NewRedisStore connects to Redis using the given URL and timeout configuration.
func NewRedisStore(redisURL string, timeouts config.TimeoutConfig) (*RedisStore, error) {
	conn, err := config.ParseRedisURL(redisURL)
	if err != nil {
		return nil, err
	}
	poolSize := config.GetEnvIntPositive("REDIS_POOL_SIZE", 20)
	minIdleConns := config.GetEnvIntPositive("REDIS_MIN_IDLE_CONNS", 5)

	opts := &redis.Options{
		Addr: conn.Addr,
		Password: conn.Password,
		PoolSize: poolSize,
		MinIdleConns: minIdleConns,
		PoolTimeout: 4 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
		ReadTimeout: timeouts.RedisReadTimeout,
		WriteTimeout: timeouts.RedisWriteTimeout,
		MaxRetries: 0,
	}
	if strings.HasPrefix(redisURL, "rediss://") {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), timeouts.RedisConnectTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisStore{
		rdb: rdb,
		cb:  resilience.NewRedisBreaker(),
	}, nil
}

// NewRedisStoreFromClient wraps an existing Redis client for tests or custom wiring.
func NewRedisStoreFromClient(rdb *redis.Client) *RedisStore {
	return &RedisStore{
		rdb: rdb,
		cb:  resilience.NewRedisBreaker(),
	}
}

// PoolStats returns the underlying Redis connection pool statistics.
func (s *RedisStore) PoolStats() *redis.PoolStats { return s.rdb.PoolStats() }

// Client exposes the underlying go-redis client for advanced use cases.
func (s *RedisStore) Client() *redis.Client { return s.rdb }

// Close shuts down the Redis client and its connection pool.
func (s *RedisStore) Close() error { return s.rdb.Close() }
