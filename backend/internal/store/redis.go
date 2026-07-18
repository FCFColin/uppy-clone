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
)

// ─── Base Redis Store ─────────────────────────────────────────────────

type baseRedisStore struct {
	rdb  *redis.Client
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps
}

func newBaseRedisStore(rdb *redis.Client, deps Deps) baseRedisStore {
	return baseRedisStore{rdb: rdb, cb: deps.RedisBreakerFactory(), deps: deps}
}

func newBaseRedisStoreWithBreaker(rdb *redis.Client, cb *gobreaker.CircuitBreaker[any], deps Deps) baseRedisStore {
	return baseRedisStore{rdb: rdb, cb: cb, deps: deps}
}

// ─── RedisStore ──────────────────────────────────────────────────────

// RedisStore wraps a go-redis client with circuit breaker protection.
//
// Domain methods are provided by embedded typed stores (SessionStore,
// RateLimitStore, RoomRegistryStore, MagicLinkStore, EmailQueueStore,
// LobbyReadCacheStore) via method promotion.
type RedisStore struct {
	rdb  *redis.Client
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps

	*SessionStore
	*RateLimitStore
	*RoomRegistryStore
	*MagicLinkStore
	*EmailQueueStore
	*LobbyReadCacheStore
}

// NewRedisStore connects to Redis using the given URL and timeout configuration.
func NewRedisStore(redisURL string, timeouts config.TimeoutConfig, deps ...Deps) (*RedisStore, error) {
	d := depsOrZero(deps...)
	conn, err := config.ParseRedisURL(redisURL)
	if err != nil {
		return nil, err
	}
	poolSize := config.GetEnvIntPositive("REDIS_POOL_SIZE", 20)
	minIdleConns := config.GetEnvIntPositive("REDIS_MIN_IDLE_CONNS", 5)

	opts := &redis.Options{
		Addr:            conn.Addr,
		Password:        conn.Password,
		PoolSize:        poolSize,
		MinIdleConns:    minIdleConns,
		PoolTimeout:     4 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
		ReadTimeout:     timeouts.RedisReadTimeout,
		WriteTimeout:    timeouts.RedisWriteTimeout,
		MaxRetries:      0,
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
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return newRedisStoreFromClient(rdb, d), nil
}

// NewRedisStoreFromClient wraps an existing Redis client for tests or custom wiring.
func NewRedisStoreFromClient(rdb *redis.Client, deps ...Deps) *RedisStore {
	return newRedisStoreFromClient(rdb, depsOrZero(deps...))
}

// newRedisStoreFromClient is the shared constructor that initializes embedded
// typed stores with a shared circuit breaker and Redis client.
func newRedisStoreFromClient(rdb *redis.Client, deps Deps) *RedisStore {
	cb := deps.RedisBreakerFactory()
	base := newBaseRedisStoreWithBreaker(rdb, cb, deps)
	return &RedisStore{
		rdb:  rdb,
		cb:   cb,
		deps: deps,

		SessionStore:        &SessionStore{baseRedisStore: base},
		RateLimitStore:      &RateLimitStore{baseRedisStore: base},
		RoomRegistryStore:   &RoomRegistryStore{baseRedisStore: base},
		MagicLinkStore:      &MagicLinkStore{baseRedisStore: base},
		EmailQueueStore:     &EmailQueueStore{baseRedisStore: base},
		LobbyReadCacheStore: &LobbyReadCacheStore{baseRedisStore: base},
	}
}

// PoolStats returns the underlying Redis connection pool statistics.
func (s *RedisStore) PoolStats() *redis.PoolStats { return s.rdb.PoolStats() }

// CircuitBreaker returns the Redis circuit breaker for degradation detection.
func (s *RedisStore) CircuitBreaker() *gobreaker.CircuitBreaker[any] { return s.cb }

// Client exposes the underlying go-redis client for advanced use cases.
func (s *RedisStore) Client() *redis.Client { return s.rdb }

// Close shuts down the Redis client and its connection pool.
func (s *RedisStore) Close() error { return s.rdb.Close() }

// ─── RedisCluster (ADR-029 domain separation) ────────────────────────

// RedisCluster holds domain-separated Redis stores for fault isolation (ADR-029).
//
// Stateful: room registry, JWT revocation, magic links, refresh tokens,
//
//	email queue, game results, outbox, Pub/Sub.
//	Requires persistence (AOF/RDB), low-latency, HA (Sentinel).
//
// Ephemeral: rate limiting.
//
//	Tolerant of data loss, can fail-open, no persistence needed.
//
// When REDIS_EPHEMERAL_URL is unset, Ephemeral falls back to Stateful
// (single-instance backward compatibility).
type RedisCluster struct {
	Stateful  *RedisStore
	Ephemeral *RedisStore
}

// NewRedisCluster creates a RedisCluster from stateful and ephemeral URLs.
// If ephemeralURL is empty, the stateful store is reused for both domains.
func NewRedisCluster(statefulURL, ephemeralURL string, timeouts config.TimeoutConfig, deps ...Deps) (*RedisCluster, error) {
	stateful, err := NewRedisStore(statefulURL, timeouts, deps...)
	if err != nil {
		return nil, err
	}

	var ephemeral *RedisStore
	if ephemeralURL == "" {
		ephemeral = stateful
	} else {
		ephemeral, err = NewRedisStore(ephemeralURL, timeouts, deps...)
		if err != nil {
			_ = stateful.Close()
			return nil, err
		}
	}

	return &RedisCluster{
		Stateful:  stateful,
		Ephemeral: ephemeral,
	}, nil
}

// IsSeparated returns true when Stateful and Ephemeral are distinct instances.
func (c *RedisCluster) IsSeparated() bool {
	return c.Stateful != c.Ephemeral
}

// Close shuts down both Redis stores. When not separated, only closes once.
func (c *RedisCluster) Close() error {
	if c.IsSeparated() {
		_ = c.Ephemeral.Close()
	}
	return c.Stateful.Close()
}

// CircuitBreakers returns circuit breakers for both stores (for health checks).
func (c *RedisCluster) CircuitBreakers() []*gobreaker.CircuitBreaker[any] {
	cbs := []*gobreaker.CircuitBreaker[any]{c.Stateful.CircuitBreaker()}
	if c.IsSeparated() {
		cbs = append(cbs, c.Ephemeral.CircuitBreaker())
	}
	return cbs
}
