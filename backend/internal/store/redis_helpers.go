package store

import (
	"context"

	"github.com/sethvargo/go-retry"
	"github.com/uppy-clone/backend/internal/resilience"
)

func (s *RedisStore) withRetryRead(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, resilience.DefaultRedisRetry(), func(ctx context.Context) error {
		_, err := s.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return resilience.MaybeRetryable(err)
	})
}

func (s *RedisStore) withCircuitBreaker(fn func() (any, error)) (any, error) {
	return s.cb.Execute(fn)
}
