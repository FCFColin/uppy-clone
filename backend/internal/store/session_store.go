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

// SessionStore handles JWT revocation and admin login tracking.
type SessionStore struct {
	baseRedisStore
}

// NewSessionStore creates a SessionStore.
func NewSessionStore(rdb *redis.Client) *SessionStore {
	return &SessionStore{baseRedisStore: newBaseRedisStore(rdb)}
}

func (s *SessionStore) RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error {
	ctx, span := telemetry.Tracer().Start(ctx, "session_store.RevokeJWT",
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

func (s *SessionStore) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "session_store.IsJWTRevoked",
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

func (s *SessionStore) IncrementFailedLogin(ctx context.Context, ip, account string) (int, int, error) {
	ipKey := adminFailKeyPrefix + ip
	acctKey := adminAcctFailPrefix + account

	pipe := s.rdb.Pipeline()
	ipCount := pipe.Incr(ctx, ipKey)
	acctCount := pipe.Incr(ctx, acctKey)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("incr failed login: %w", err)
	}

	ipVal, _ := ipCount.Result()
	acctVal, _ := acctCount.Result()
	if ipVal == 1 {
		s.rdb.Expire(ctx, ipKey, 15*time.Minute)
	}
	if acctVal == 1 {
		s.rdb.Expire(ctx, acctKey, 15*time.Minute)
	}
	return int(ipVal), int(acctVal), nil
}

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

func (s *SessionStore) AddAdminJTI(ctx context.Context, jti string, ttl time.Duration) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.SAdd(ctx, adminActiveJTISetKey, jti).Err(); err != nil {
			return nil, fmt.Errorf("add admin jti: %w", err)
		}
		if err := s.rdb.Expire(ctx, adminActiveJTISetKey, ttl).Err(); err != nil {
			return nil, fmt.Errorf("set admin jti ttl: %w", err)
		}
		return nil, nil
	})
	return err
}

func (s *SessionStore) RemoveAdminJTI(ctx context.Context, jti string) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.SRem(ctx, adminActiveJTISetKey, jti).Err(); err != nil {
			return nil, fmt.Errorf("remove admin jti: %w", err)
		}
		return nil, nil
	})
	return err
}

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
