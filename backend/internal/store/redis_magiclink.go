package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sethvargo/go-retry"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (s *RedisStore) StoreMagicToken(ctx context.Context, hashedToken string, data []byte, ttl time.Duration) error {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.StoreMagicToken",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SET")),
	)
	defer span.End()

	key := magicTokenKey(hashedToken)
	_, err := s.cb.Execute(func() (any, error) {
		if setErr := s.rdb.Set(ctx, key, data, ttl).Err(); setErr != nil {
			return nil, fmt.Errorf("store magic token: %w", setErr)
		}
		return nil, nil
	})
	return err
}

func (s *RedisStore) GetMagicToken(ctx context.Context, hashedToken string) ([]byte, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.GetMagicToken",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "GET")),
	)
	defer span.End()

	key := magicTokenKey(hashedToken)
	var result []byte
	err := retry.Do(ctx, resilience.DefaultRedisRetry(), func(ctx context.Context) error {
		_, cbErr := s.cb.Execute(func() (any, error) {
			val, getErr := s.rdb.Get(ctx, key).Bytes()
			if getErr != nil {
				if getErr == redis.Nil {
					return nil, nil
				}
				return nil, fmt.Errorf("get magic token: %w", getErr)
			}
			result = val
			return nil, nil
		})
		return resilience.MaybeRetryable(cbErr)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *RedisStore) DeleteMagicToken(ctx context.Context, hashedToken string) error {
	key := magicTokenKey(hashedToken)
	_, err := s.cb.Execute(func() (any, error) {
		if delErr := s.rdb.Del(ctx, key).Err(); delErr != nil {
			return nil, fmt.Errorf("delete magic token: %w", delErr)
		}
		return nil, nil
	})
	return err
}

func magicTokenKey(hash string) string { return "magic:" + hash }
