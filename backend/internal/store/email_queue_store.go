package store

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// EmailQueueStore handles email and game result queue operations via Redis Streams.
type EmailQueueStore struct {
	baseRedisStore
}

// NewEmailQueueStore creates an EmailQueueStore.
func NewEmailQueueStore(rdb *redis.Client) *EmailQueueStore {
	return &EmailQueueStore{baseRedisStore: newBaseRedisStore(rdb)}
}

func (s *EmailQueueStore) EnqueueEmail(ctx context.Context, payload []byte) error {
	ctx, span := telemetry.Tracer().Start(ctx, "email_queue.EnqueueEmail",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "XADD"),
		),
	)
	defer span.End()

	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream:   "email:queue",
			MaxLen:   100_000,
			Approx:   true,
			Values:   map[string]interface{}{"payload": payload},
		}).Err(); err != nil {
			return nil, fmt.Errorf("enqueue email: %w", err)
		}
		return nil, nil
	})
	return err
}

func (s *EmailQueueStore) EnqueueGameResult(ctx context.Context, payload []byte) error {
	ctx, span := telemetry.Tracer().Start(ctx, "email_queue.EnqueueGameResult",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "XADD"),
		),
	)
	defer span.End()

	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream:   "game:results",
			MaxLen:   100_000,
			Approx:   true,
			Values:   map[string]interface{}{"payload": payload},
		}).Err(); err != nil {
			return nil, fmt.Errorf("enqueue game result: %w", err)
		}
		return nil, nil
	})
	return err
}
