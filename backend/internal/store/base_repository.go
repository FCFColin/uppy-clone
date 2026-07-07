package store

import (
	"context"

	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/resilience"
)

// baseRepository provides shared pool and circuit breaker for domain repositories.
type baseRepository struct {
	pool pgPool
	cb   *gobreaker.CircuitBreaker[any]
}

func newBaseRepository(pool pgPool) baseRepository {
	cb := resilience.NewPostgresBreaker()
	return baseRepository{pool: pool, cb: cb}
}

func (b *baseRepository) withRetryRead(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, err := b.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return resilience.MaybeRetryable(err)
	})
}

func (b *baseRepository) withRetryWrite(ctx context.Context, fn func(context.Context) error) error {
	_, err := b.cb.Execute(func() (any, error) {
		return nil, fn(ctx)
	})
	return err
}


