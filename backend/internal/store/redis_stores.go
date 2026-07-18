package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sethvargo/go-retry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ─── Key Helpers & Lua Scripts ────────────────────────────────────────

const (
	adminFailKeyPrefix      = "admin:login:fail:"
	adminLockKeyPrefix      = "admin:login:lock:"
	adminAcctFailPrefix     = "admin:login:acct_fail:"
	adminAcctLockPrefix     = "admin:login:acct_lock:"
	adminActiveJTISetKey    = "admin:active-jtis"
	adminActiveJTIKeyPrefix = "admin:jti:"
)

func jwtRevokedKey(jti string) string { return "jwt_revoked:" + jti }

// consumeMagicTokenScript atomically GETs and DELETEs a magic link token.
var consumeMagicTokenScript = redis.NewScript(`
	local val = redis.call('GET', KEYS[1])
	if val then
		redis.call('DEL', KEYS[1])
	end
	return val
`)

func magicTokenKey(hash string) string { return "magic:" + hash }

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

func roomInfoKey(code string) string { return "room:" + code }

// roomIndexKey returns the Redis SET key used as an O(1) index of all active
// room codes, replacing the O(N) full-keyspace SCAN (store-015).
func roomIndexKey() string { return "room:index" }

// ─── SessionStore (JWT revocation + admin login tracking) ────────────

var incrementFailedLoginScript = redis.NewScript(`
local ipCount = redis.call('INCR', KEYS[1])
local acctCount = redis.call('INCR', KEYS[2])
if ipCount == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
if acctCount == 1 then
    redis.call('EXPIRE', KEYS[2], ARGV[1])
end
return {ipCount, acctCount}
`)

// SessionStore handles JWT revocation and admin login tracking.
type SessionStore struct {
	baseRedisStore
}

// NewSessionStore creates a SessionStore.
func NewSessionStore(rdb *redis.Client, deps ...Deps) *SessionStore {
	d := depsOrZero(deps...)
	return &SessionStore{baseRedisStore: newBaseRedisStore(rdb, d)}
}

// RevokeJWT adds a JWT ID to the revocation list with the given TTL.
func (s *SessionStore) RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error {
	ctx, span := s.deps.Tracer.Start(ctx, "session_store.RevokeJWT",
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

// IsJWTRevoked checks whether a JWT ID has been revoked.
func (s *SessionStore) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "session_store.IsJWTRevoked",
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

// IncrementFailedLogin increments failed login counts for the given IP and account.
func (s *SessionStore) IncrementFailedLogin(ctx context.Context, ip, account string) (int, int, error) {
	ipKey := adminFailKeyPrefix + ip
	acctKey := adminAcctFailPrefix + account

	var ipVal, acctVal int64
	_, err := s.cb.Execute(func() (any, error) {
		result, scriptErr := incrementFailedLoginScript.Run(ctx, s.rdb, []string{ipKey, acctKey}, 900).Result()
		if scriptErr != nil {
			return nil, fmt.Errorf("incr failed login: %w", scriptErr)
		}
		vals, ok := result.([]interface{})
		if !ok || len(vals) < 2 {
			return nil, fmt.Errorf("incr failed login: unexpected result type")
		}
		ipVal, ok = vals[0].(int64)
		if !ok {
			return nil, fmt.Errorf("incr failed login: unexpected ip count type")
		}
		acctVal, ok = vals[1].(int64)
		if !ok {
			return nil, fmt.Errorf("incr failed login: unexpected acct count type")
		}
		return nil, nil
	})
	if err != nil {
		return 0, 0, err
	}
	return int(ipVal), int(acctVal), nil
}

// IsLoginLocked checks whether login is locked for the given IP or account.
func (s *SessionStore) IsLoginLocked(ctx context.Context, ip, account string) (bool, error) {
	ipKey := adminLockKeyPrefix + ip
	acctKey := adminAcctLockPrefix + account

	pipe := s.rdb.Pipeline()
	ipLock := pipe.Exists(ctx, ipKey)
	acctLock := pipe.Exists(ctx, acctKey)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("check login lock: %w", err)
	}

	ipLocked, _ := ipLock.Result()
	acctLocked, _ := acctLock.Result()
	return ipLocked > 0 || acctLocked > 0, nil
}

// SetLoginLock sets login lock keys for the given IP and account with a TTL.
func (s *SessionStore) SetLoginLock(ctx context.Context, ip, account string, ttl time.Duration) error {
	ipKey := adminLockKeyPrefix + ip
	acctKey := adminAcctLockPrefix + account

	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, ipKey, "1", ttl)
	pipe.Set(ctx, acctKey, "1", ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("set login lock: %w", err)
	}
	return nil
}

// ResetFailedLogin clears failed-login counters and locks for the given IP and account.
func (s *SessionStore) ResetFailedLogin(ctx context.Context, ip, account string) error {
	ipFailKey := adminFailKeyPrefix + ip
	ipLockKey := adminLockKeyPrefix + ip
	acctFailKey := adminAcctFailPrefix + account
	acctLockKey := adminAcctLockPrefix + account

	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, ipFailKey, ipLockKey)
	pipe.Del(ctx, acctFailKey, acctLockKey)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("reset failed login: %w", err)
	}
	return nil
}

// AddAdminJTI adds an admin JWT ID to the active set with a TTL.
func (s *SessionStore) AddAdminJTI(ctx context.Context, jti string, ttl time.Duration) error {
	_, err := s.cb.Execute(func() (any, error) {
		pipe := s.rdb.Pipeline()
		pipe.SAdd(ctx, adminActiveJTISetKey, jti)
		pipe.Expire(ctx, adminActiveJTISetKey, ttl)
		_, err := pipe.Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("add admin jti: %w", err)
		}
		return nil, nil
	})
	return err
}

// RemoveAdminJTI removes an admin JWT ID from the active set.
func (s *SessionStore) RemoveAdminJTI(ctx context.Context, jti string) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.SRem(ctx, adminActiveJTISetKey, jti).Err(); err != nil {
			return nil, fmt.Errorf("remove admin jti: %w", err)
		}
		return nil, nil
	})
	return err
}

// GetAllAdminJTIs returns all active admin JWT IDs.
func (s *SessionStore) GetAllAdminJTIs(ctx context.Context) ([]string, error) {
	var jtis []string
	_, err := s.cb.Execute(func() (any, error) {
		var getErr error
		jtis, getErr = s.rdb.SMembers(ctx, adminActiveJTISetKey).Result()
		if getErr != nil {
			return nil, fmt.Errorf("get all admin jtis: %w", getErr)
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return jtis, nil
}

// ─── MagicLinkStore ──────────────────────────────────────────────────

// MagicLinkStore handles magic-link token persistence in Redis.
type MagicLinkStore struct {
	baseRedisStore
}

// NewMagicLinkStore creates a MagicLinkStore.
func NewMagicLinkStore(rdb *redis.Client, deps ...Deps) *MagicLinkStore {
	d := depsOrZero(deps...)
	return &MagicLinkStore{baseRedisStore: newBaseRedisStore(rdb, d)}
}

// StoreMagicToken stores a hashed magic link token with a TTL.
func (s *MagicLinkStore) StoreMagicToken(ctx context.Context, hashedToken string, data []byte, ttl time.Duration) error {
	ctx, span := s.deps.Tracer.Start(ctx, "magiclink_store.StoreMagicToken",
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

// GetMagicToken retrieves a magic link token's data by hash.
func (s *MagicLinkStore) GetMagicToken(ctx context.Context, hashedToken string) ([]byte, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "magiclink_store.GetMagicToken",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "GET")),
	)
	defer span.End()

	key := magicTokenKey(hashedToken)
	var result []byte
	err := retry.Do(ctx, s.deps.RedisRetryPolicy, func(ctx context.Context) error {
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
		return s.deps.MaybeRetryableFn(cbErr)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ConsumeMagicToken atomically retrieves and deletes a magic link token.
func (s *MagicLinkStore) ConsumeMagicToken(ctx context.Context, tokenHash string) ([]byte, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "magiclink_store.ConsumeMagicToken",
		trace.WithAttributes(attribute.String("db.system", "redis"),
			attribute.String("db.operation", "EVAL")),
	)
	defer span.End()

	key := magicTokenKey(tokenHash)
	var result []byte
	_, err := s.cb.Execute(func() (any, error) {
		val, err := consumeMagicTokenScript.Run(ctx, s.rdb, []string{key}).Result()
		if err != nil {
			return nil, fmt.Errorf("consume magic token: %w", err)
		}
		if val == nil {
			return nil, nil
		}
		valStr, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("consume magic token: unexpected result type %T", val)
		}
		result = []byte(valStr)
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteMagicToken removes a magic link token from Redis.
func (s *MagicLinkStore) DeleteMagicToken(ctx context.Context, hashedToken string) error {
	key := magicTokenKey(hashedToken)
	_, err := s.cb.Execute(func() (any, error) {
		if delErr := s.rdb.Del(ctx, key).Err(); delErr != nil {
			return nil, fmt.Errorf("delete magic token: %w", delErr)
		}
		return nil, nil
	})
	return err
}

// ─── RateLimitStore ──────────────────────────────────────────────────

// RateLimitStore handles rate limiting via Redis.
type RateLimitStore struct {
	baseRedisStore
}

// NewRateLimitStore creates a RateLimitStore.
func NewRateLimitStore(rdb *redis.Client, deps ...Deps) *RateLimitStore {
	d := depsOrZero(deps...)
	return &RateLimitStore{baseRedisStore: newBaseRedisStore(rdb, d)}
}

// CheckRateLimit checks and increments the rate limit counter for the given key.
func (s *RateLimitStore) CheckRateLimit(ctx context.Context, key string, maxCount int64, window time.Duration) (bool, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "ratelimit_store.CheckRateLimit",
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

// ─── EmailQueueStore ─────────────────────────────────────────────────

// EmailQueueStore handles email and game result queue operations via Redis Streams.
type EmailQueueStore struct {
	baseRedisStore
}

// NewEmailQueueStore creates an EmailQueueStore.
func NewEmailQueueStore(rdb *redis.Client, deps ...Deps) *EmailQueueStore {
	d := depsOrZero(deps...)
	return &EmailQueueStore{baseRedisStore: newBaseRedisStore(rdb, d)}
}

// EnqueueEmail adds an email payload to the Redis stream queue.
func (s *EmailQueueStore) EnqueueEmail(ctx context.Context, payload []byte) error {
	ctx, span := s.deps.Tracer.Start(ctx, "email_queue.EnqueueEmail",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "XADD"),
		),
	)
	defer span.End()

	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "email:queue",
			MaxLen: 100_000,
			Approx: true,
			Values: map[string]interface{}{"payload": payload},
		}).Err(); err != nil {
			return nil, fmt.Errorf("enqueue email: %w", err)
		}
		return nil, nil
	})
	return err
}

// ─── Multi-IP Tracking (RO-046) ──────────────────────────────────────

// multiIPLoginScript is a Lua script that atomically adds an IP to the user's
// IP set, refreshes the TTL, and returns the cardinality — all in a single
// Redis round-trip (auth-009: was 3 separate calls → 1).
var multiIPLoginScript = redis.NewScript(`
redis.call('SADD', KEYS[1], ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[2])
return redis.call('SCARD', KEYS[1])
`)

// TrackUserIPs adds clientIP to the user's IP set with a 1-hour TTL and
// returns the count of distinct IPs seen in the window. This is the pure
// Redis Lua operation extracted from the auth package (RO-046).
func TrackUserIPs(ctx context.Context, rdb redis.Scripter, userID, clientIP string) (int64, error) {
	ipKey := "user:ips:" + userID
	return multiIPLoginScript.Run(ctx, rdb, []string{ipKey}, clientIP, int(time.Hour.Seconds())).Int64()
}
