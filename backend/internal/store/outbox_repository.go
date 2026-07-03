package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRepository handles transactional outbox event persistence.
type OutboxRepository struct {
	pool pgPool
}

// NewOutboxRepository creates an OutboxRepository.
func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

func (r *OutboxRepository) InsertOutboxEvent(ctx context.Context, aggregateType, aggregateID string, payload []byte) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, payload) VALUES ($1, $2, $3)`,
		aggregateType, aggregateID, payload)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}
