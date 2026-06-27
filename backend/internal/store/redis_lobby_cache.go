package store

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func (s *RedisStore) EnqueueEmail(ctx context.Context, payload []byte) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "email:queue",
			Values: map[string]interface{}{"payload": payload},
		}).Err(); err != nil {
			return nil, fmt.Errorf("enqueue email: %w", err)
		}
		return nil, nil
	})
	return err
}

func (s *RedisStore) EnqueueGameResult(ctx context.Context, payload []byte) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "game:results",
			Values: map[string]interface{}{"payload": payload},
		}).Err(); err != nil {
			return nil, fmt.Errorf("enqueue game result: %w", err)
		}
		return nil, nil
	})
	return err
}
