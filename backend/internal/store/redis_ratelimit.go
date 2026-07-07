package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
local is_first = 0
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
    is_first = 1
end
return {count, is_first}
`)

func rateLimitKey(key string) string { return "rl:" + key }

func (s *RedisStore) CheckRateLimit(ctx context.Context, key string, maxCount int64, window time.Duration) (bool, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "ratelimit_store.CheckRateLimit",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "INCR")),
	)
	defer span.End()

	rk := rateLimitKey(key)
	var allowed bool
	_, err := s.cb.Execute(func() (any, error) {
		result, scriptErr := rateLimitScript.Run(ctx, s.rdb, []string{rk}, int(window.Seconds())).Result()
		if scriptErr != nil {
			return nil, fmt.Errorf("rate limit script: %w", scriptErr)
		}
		vals, ok := result.([]interface{})
		if !ok || len(vals) < 1 {
			return nil, fmt.Errorf("rate limit script: unexpected result type")
		}
		count, ok := vals[0].(int64)
		if !ok {
			return nil, fmt.Errorf("rate limit script: unexpected count type")
		}
		allowed = count <= maxCount
		return nil, nil
	})
	if err != nil {
		return false, err
	}
	return allowed, nil
}