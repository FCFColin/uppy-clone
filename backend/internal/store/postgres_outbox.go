package store

import (
	"context"
	"fmt"
)

// InsertOutboxEvent writes a domain event to outbox_events for async publish.
func (s *PostgresStore) InsertOutboxEvent(ctx context.Context, aggregateType, aggregateID string, payload []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, payload) VALUES ($1, $2, $3)`,
		aggregateType, aggregateID, payload)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}
