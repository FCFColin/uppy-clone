package store

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// EnqueueEmail pushes an email job onto the Redis Stream outbox.
func (s *RedisStore) EnqueueEmail(ctx context.Context, payload []byte) error {
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

// EnqueueGameResult pushes a game result job onto the Redis Stream outbox.
func (s *RedisStore) EnqueueGameResult(ctx context.Context, payload []byte) error {
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
