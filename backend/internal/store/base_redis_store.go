package store

import (
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/resilience"
)

// baseRedisStore provides shared Redis client and circuit breaker for domain stores.
type baseRedisStore struct {
	rdb *redis.Client
	cb  *gobreaker.CircuitBreaker[any]
}

func newBaseRedisStore(rdb *redis.Client) baseRedisStore {
	return baseRedisStore{
		rdb: rdb,
		cb:  resilience.NewRedisBreaker(),
	}
}
