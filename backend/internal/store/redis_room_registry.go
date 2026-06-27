package store

import (
	"context"
	"fmt"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (s *RedisStore) RegisterRoom(ctx context.Context, code string, data []byte, ttl time.Duration) error {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.RegisterRoom",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "PIPELINE_SET_SADD")),
	)
	defer span.End()

	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		pipe := s.rdb.Pipeline()
		pipe.Set(ctx, key, data, ttl)
		pipe.SAdd(ctx, "rooms:active", code)
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			return nil, fmt.Errorf("register room: %w", execErr)
		}
		return nil, nil
	})
	return err
}

func (s *RedisStore) UnregisterRoom(ctx context.Context, code string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.UnregisterRoom",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "PIPELINE_DEL_SREM")),
	)
	defer span.End()

	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		pipe := s.rdb.Pipeline()
		pipe.Del(ctx, key)
		pipe.SRem(ctx, "rooms:active", code)
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			return nil, fmt.Errorf("unregister room: %w", execErr)
		}
		return nil, nil
	})
	return err
}

func (s *RedisStore) ListActiveRooms(ctx context.Context) ([]string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.ListActiveRooms",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SMEMBERS")),
	)
	defer span.End()

	var result []string
	err := retry.Do(ctx, resilience.DefaultRedisRetry(), func(ctx context.Context) error {
		_, cbErr := s.cb.Execute(func() (any, error) {
			vals, sMembersErr := s.rdb.SMembers(ctx, "rooms:active").Result()
			if sMembersErr != nil {
				return nil, fmt.Errorf("list active rooms: %w", sMembersErr)
			}
			result = vals
			return nil, nil
		})
		return resilience.MaybeRetryable(cbErr)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func roomInfoKey(code string) string { return "room:" + code }
