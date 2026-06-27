package store

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/resilience"
)

type RedisStore struct {
	rdb *redis.Client
	cb  *gobreaker.CircuitBreaker[any]
}

func NewRedisStore(addr string, timeouts config.TimeoutConfig) (*RedisStore, error) {
	poolSize := getEnvInt("REDIS_POOL_SIZE", 20)
	minIdleConns := getEnvInt("REDIS_MIN_IDLE_CONNS", 5)

	rdb := redis.NewClient(&redis.Options{
		Addr:            addr,
		PoolSize:        poolSize,
		MinIdleConns:    minIdleConns,
		PoolTimeout:     4 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
		ReadTimeout:     timeouts.RedisReadTimeout,
		WriteTimeout:    timeouts.RedisWriteTimeout,
		MaxRetries:      0,
	})

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

func (s *RedisStore) PoolStats() *redis.PoolStats { return s.rdb.PoolStats() }
func (s *RedisStore) Client() *redis.Client       { return s.rdb }
func (s *RedisStore) Close() error                { return s.rdb.Close() }

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
