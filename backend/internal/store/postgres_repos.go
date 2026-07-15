package store

import (
	"context"
	"fmt"

	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
)

// ─── Base Repository ─────────────────────────────────────────────────

type baseRepository struct {
	pool pgPool
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps
}

func newBaseRepository(pool pgPool, deps Deps) baseRepository {
	return baseRepository{pool: pool, cb: deps.PostgresBreakerFactory(), deps: deps}
}

func (b *baseRepository) withRetryRead(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, b.deps.DBRetryPolicy, func(ctx context.Context) error {
		_, err := b.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return b.deps.MaybeRetryableFn(err)
	})
}

func (b *baseRepository) withRetryWrite(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, b.deps.DBRetryPolicy, func(ctx context.Context) error {
		_, err := b.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return b.deps.MaybeRetryableFn(err)
	})
}

// ─── Outbox Repository ───────────────────────────────────────────────

// OutboxRepository handles transactional outbox event persistence.
type OutboxRepository struct {
	pool pgPool
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps
}

// NewOutboxRepository creates an OutboxRepository.
func NewOutboxRepository(pool pgPool, deps ...Deps) *OutboxRepository {
	d := depsOrZero(deps...)
	return &OutboxRepository{pool: pool, cb: d.PostgresBreakerFactory(), deps: d}
}

// InsertOutboxEvent inserts an outbox event for the transactional outbox pattern.
func (r *OutboxRepository) InsertOutboxEvent(ctx context.Context, aggregateType, aggregateID string, payload []byte) error {
	_, err := r.cb.Execute(func() (any, error) {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO outbox_events (aggregate_type, aggregate_id, payload) VALUES ($1, $2, $3)`,
			aggregateType, aggregateID, payload)
		if err != nil {
			return nil, fmt.Errorf("insert outbox event: %w", err)
		}
		return nil, nil
	})
	return err
}
