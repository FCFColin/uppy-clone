package store

import (
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/config"
)

// RedisCluster holds domain-separated Redis stores for fault isolation (ADR-029).
//
// Stateful: room registry, JWT revocation, magic links, refresh tokens,
//           email queue, game results, outbox, Pub/Sub.
//           Requires persistence (AOF/RDB), low-latency, HA (Sentinel).
//
// Ephemeral: rate limiting, idempotency cache.
//            Tolerant of data loss, can fail-open, no persistence needed.
//
// When REDIS_EPHEMERAL_URL is unset, Ephemeral falls back to Stateful
// (single-instance backward compatibility).
type RedisCluster struct {
	Stateful  *RedisStore
	Ephemeral *RedisStore
}

// NewRedisCluster creates a RedisCluster from stateful and ephemeral URLs.
// If ephemeralURL is empty, the stateful store is reused for both domains.
func NewRedisCluster(statefulURL, ephemeralURL string, timeouts config.TimeoutConfig) (*RedisCluster, error) {
	stateful, err := NewRedisStore(statefulURL, timeouts)
	if err != nil {
		return nil, err
	}

	var ephemeral *RedisStore
	if ephemeralURL == "" {
		ephemeral = stateful
	} else {
		ephemeral, err = NewRedisStore(ephemeralURL, timeouts)
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

// NewRedisClusterFromStores wraps existing RedisStore instances (for tests).
func NewRedisClusterFromStores(stateful, ephemeral *RedisStore) *RedisCluster {
	if ephemeral == nil {
		ephemeral = stateful
	}
	return &RedisCluster{
		Stateful:  stateful,
		Ephemeral: ephemeral,
	}
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
