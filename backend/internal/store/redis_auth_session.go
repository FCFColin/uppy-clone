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

// RevokeJWT stores a JWT jti in the revocation list until TTL expires.
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

// IsJWTRevoked reports whether a JWT jti has been revoked.
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

// IncrementFailedLogin increments the failed admin login counter for an IP.
func (s *RedisStore) IncrementFailedLogin(ctx context.Context, ip string) (int, error) {
	key := "admin:login:fail:" + ip
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("incr failed login: %w", err)
	}
	if count == 1 {
		s.rdb.Expire(ctx, key, 15*time.Minute)
	}
	return int(count), nil
}

// IsLoginLocked reports whether an IP is locked out from admin login.
func (s *RedisStore) IsLoginLocked(ctx context.Context, ip string) (bool, error) {
	key := "admin:login:lock:" + ip
	val, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("check login lock: %w", err)
	}
	return val > 0, nil
}

// SetLoginLock locks admin login for an IP until TTL expires.
func (s *RedisStore) SetLoginLock(ctx context.Context, ip string, ttl time.Duration) error {
	key := "admin:login:lock:" + ip
	if err := s.rdb.Set(ctx, key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("set login lock: %w", err)
	}
	return nil
}

// ResetFailedLogin clears the failed admin login counter for an IP.
func (s *RedisStore) ResetFailedLogin(ctx context.Context, ip string) error {
	failKey := "admin:login:fail:" + ip
	lockKey := "admin:login:lock:" + ip
	return s.rdb.Del(ctx, failKey, lockKey).Err()
}

func jwtRevokedKey(jti string) string { return "jwt_revoked:" + jti }
