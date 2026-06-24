package store

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RedisStore provides Redis-backed ephemeral storage.
type RedisStore struct {
	rdb *redis.Client
	cb  *gobreaker.CircuitBreaker[any]
}

// NewRedisStore creates a Redis client and validates connectivity.
//
// 企业为何需要：默认连接池配置（PoolSize=10*runtime.GOMAXPROCS）在高并发下可能不足。显式配置确保可预测的行为。
func NewRedisStore(addr string, timeouts config.TimeoutConfig) (*RedisStore, error) {
	poolSize := getEnvInt("REDIS_POOL_SIZE", 20)
	minIdleConns := getEnvInt("REDIS_MIN_IDLE_CONNS", 5)

	rdb := redis.NewClient(&redis.Options{
		Addr:            addr,
		PoolSize:        poolSize,              // Match PG MaxConns
		MinIdleConns:    minIdleConns,          // Keep warm connections
		PoolTimeout:     4 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
		ReadTimeout:     timeouts.RedisReadTimeout,
		WriteTimeout:    timeouts.RedisWriteTimeout,
		MaxRetries:      0,                     // We handle retries ourselves
	})

	ctx, cancel := context.WithTimeout(context.Background(), timeouts.RedisConnectTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisStore{
		rdb: rdb,
		cb:  resilience.NewRedisBreaker(),
	}, nil
}

// PoolStats returns the current Redis connection pool statistics.
func (s *RedisStore) PoolStats() *redis.PoolStats {
	return s.rdb.PoolStats()
}

// Client returns the underlying Redis client for health checks.
func (s *RedisStore) Client() *redis.Client {
	return s.rdb
}

// Close releases the Redis client.
func (s *RedisStore) Close() error {
	return s.rdb.Close()
}

// --- Magic Link tokens ---

// StoreMagicToken stores a hashed magic-link token with TTL.
// NOT retried: write operation — retrying could create duplicate tokens with different TTLs.
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

// GetMagicToken retrieves the data associated with a hashed magic-link token.
// Returns nil if the token does not exist or has expired.
// Safe to retry: read-only, idempotent operation.
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

// DeleteMagicToken removes a magic-link token.
// NOT retried: delete is one-time-use semantics — retrying a failed delete could
// leave an already-consumed token alive, but the risk of double-delete side effects
// outweighs the benefit. The token will expire via TTL anyway.
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

// --- Rate Limiting ---

// rateLimitScript atomically increments and sets expiry.
// P4-4: Lua 脚本保证 INCR+EXPIRE 原子性，防止 INCR 成功但 EXPIRE 失败导致
// 限流计数器永不过期的竞态条件。
// Returns: {count, is_first}
var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
local is_first = 0
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
    is_first = 1
end
return {count, is_first}
`)

// CheckRateLimit implements a sliding-window counter.
// Returns true if the request is allowed (count <= maxCount within window).
func (s *RedisStore) CheckRateLimit(ctx context.Context, key string, maxCount int64, window time.Duration) (bool, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.CheckRateLimit",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "INCR")),
	)
	defer span.End()

	rk := rateLimitKey(key)
	var allowed bool
	_, err := s.cb.Execute(func() (any, error) {
		result, err := rateLimitScript.Run(ctx, s.rdb, []string{rk}, int(window.Seconds())).Result()
		if err != nil {
			return nil, fmt.Errorf("rate limit script: %w", err)
		}
		// result is []interface{}{count, is_first}
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

// --- Room Registry (multi-instance) ---

// RegisterRoom adds a room to the active rooms set and stores its metadata.
// NOT retried: pipeline SET+SADD is not idempotent — retrying could add duplicate
// entries to the active rooms set.
// Enterprise rationale: Room state in Redis enables multi-instance deployment.
// When Hub instance A creates a room, instance B can discover it via Redis.
// This is the first step toward horizontal scaling per ADR-005.
// Trade-off: Extra Redis round-trip per room create/destroy, but enables
// future multi-instance deployment.
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

// UnregisterRoom removes a room from the active rooms set and deletes its metadata.
// NOT retried: pipeline DEL+SREM — retrying could remove a re-registered room.
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

// ListActiveRooms returns all active room codes from the Redis set.
// Safe to retry: read-only, idempotent operation.
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

// --- JWT Revocation List ---

// 企业为何需要：无撤销机制的 JWT 意味着被盗 token 在过期前持续有效。JWT 撤销列表是登出安全的行业标准实现，
// 用 Redis SET + TTL 实现最小性能开销。

// RevokeJWT adds a JWT's jti to the revocation list with the given TTL.
// The TTL should be set to the remaining lifetime of the access token so that
// revoked entries are automatically cleaned up when the token would have expired anyway.
func (s *RedisStore) RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.RevokeJWT",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SET")),
	)
	defer span.End()

	key := jwtRevokedKey(jti)
	_, err := s.cb.Execute(func() (any, error) {
		if setErr := s.rdb.Set(ctx, key, "1", ttl).Err(); setErr != nil {
			return nil, fmt.Errorf("revoke jwt: %w", setErr)
		}
		return nil, nil
	})
	return err
}

// IsJWTRevoked checks if a JWT's jti is in the revocation list.
// Returns true if the jti has been revoked, false otherwise.
func (s *RedisStore) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "redis.IsJWTRevoked",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "GET")),
	)
	defer span.End()

	key := jwtRevokedKey(jti)
	var revoked bool
	_, err := s.cb.Execute(func() (any, error) {
		val, getErr := s.rdb.Get(ctx, key).Result()
		if getErr != nil {
			if getErr == redis.Nil {
				revoked = false
				return nil, nil
			}
			return nil, fmt.Errorf("check jwt revoked: %w", getErr)
		}
		revoked = val != ""
		return nil, nil
	})
	if err != nil {
		return false, err
	}
	return revoked, nil
}

// --- Admin Login Lockout ---
//
// 企业为何需要：无锁定机制的登录接口可被暴力破解。基于 IP 的失败计数 + 锁定
// 是暴力破解防御的行业标准。Redis TTL 自动解除锁定，无需后台清理任务。

// IncrementFailedLogin increments the failed login counter for an IP.
// Returns the current count after increment. On the first failure, sets a
// 15-minute TTL on the counter so the window slides forward.
// NOT retried: INCR is atomic and idempotent per-call; retrying could inflate the count.
func (s *RedisStore) IncrementFailedLogin(ctx context.Context, ip string) (int, error) {
	key := "admin:login:fail:" + ip
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("incr failed login: %w", err)
	}
	if count == 1 {
		// Set TTL on first failure — 15 minute sliding window
		s.rdb.Expire(ctx, key, 15*time.Minute)
	}
	return int(count), nil
}

// IsLoginLocked checks if the IP is currently locked out.
// Safe to retry: read-only, idempotent operation.
func (s *RedisStore) IsLoginLocked(ctx context.Context, ip string) (bool, error) {
	key := "admin:login:lock:" + ip
	val, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("check login lock: %w", err)
	}
	return val > 0, nil
}

// SetLoginLock sets the lockout key for an IP with the given TTL.
// Called by the handler when failed attempts reach the threshold (5).
func (s *RedisStore) SetLoginLock(ctx context.Context, ip string, ttl time.Duration) error {
	key := "admin:login:lock:" + ip
	if err := s.rdb.Set(ctx, key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("set login lock: %w", err)
	}
	return nil
}

// ResetFailedLogin resets the failed login counter for an IP (on successful login).
// Deletes both the fail counter and the lock key.
func (s *RedisStore) ResetFailedLogin(ctx context.Context, ip string) error {
	failKey := "admin:login:fail:" + ip
	lockKey := "admin:login:lock:" + ip
	return s.rdb.Del(ctx, failKey, lockKey).Err()
}

// --- Async Queues (Redis Streams) ---

// EnqueueEmail adds an email payload to the email queue (Redis Stream).
// 企业为何需要：异步邮件发送避免 SMTP/HTTP 延迟阻塞请求，Redis Stream 提供持久化与消费者组语义。
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

// EnqueueGameResult adds a game result payload to the game:results Redis Stream.
// 企业为何需要：游戏结束热路径不应被 PG 写入延迟阻塞，异步队列削峰填谷。
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

// --- Key helpers ---

func magicTokenKey(hash string) string {
	return "magic:" + hash
}

func rateLimitKey(key string) string {
	return "rl:" + key
}

func roomInfoKey(code string) string {
	return "room:" + code
}

func jwtRevokedKey(jti string) string {
	return "jwt_revoked:" + jti
}

// getEnvInt returns the environment variable value as int, or a default.
// Invalid or non-positive values fall back to defaultVal.
func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
