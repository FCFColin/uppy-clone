package store

import (
	"context"
	"fmt"

	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/resilience"
)

// OutboxRepository handles transactional outbox event persistence.
type OutboxRepository struct {
	pool pgPool
	cb   *gobreaker.CircuitBreaker[any]
}

// NewOutboxRepository creates an OutboxRepository.
func NewOutboxRepository(pool pgPool) *OutboxRepository {
	return &OutboxRepository{pool: pool, cb: resilience.NewPostgresBreaker()}
}

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