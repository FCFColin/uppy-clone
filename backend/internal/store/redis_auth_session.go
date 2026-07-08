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

const (
	adminFailKeyPrefix    = "admin:login:fail:"
	adminLockKeyPrefix    = "admin:login:lock:"
	adminAcctFailPrefix   = "admin:login:acct_fail:"
	adminAcctLockPrefix   = "admin:login:acct_lock:"
	adminActiveJTISetKey  = "admin:active-jtis"
	adminActiveJTIKeyPrefix = "admin:jti:"
)

func jwtRevokedKey(jti string) string { return "jwt_revoked:" + jti }

func (s *RedisStore) RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error {
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

func (s *RedisStore) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
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

func (s *RedisStore) IncrementFailedLogin(ctx context.Context, ip, account string) (int, int, error) {
	ipKey := adminFailKeyPrefix + ip
	acctKey := adminAcctFailPrefix + account

	var ipVal, acctVal int64
	_, err := s.cb.Execute(func() (any, error) {
		pipe := s.rdb.Pipeline()
		ipCounter := pipe.Incr(ctx, ipKey)
		acctCounter := pipe.Incr(ctx, acctKey)
		_, execErr := pipe.Exec(ctx)
		if execErr != nil {
			return nil, fmt.Errorf("incr failed login: %w", execErr)
		}
		ipVal, _ = ipCounter.Result()
		acctVal, _ = acctCounter.Result()
		if ipVal == 1 {
			s.rdb.Expire(ctx, ipKey, 15*time.Minute)
		}
		if acctVal == 1 {
			s.rdb.Expire(ctx, acctKey, 15*time.Minute)
		}
		return nil, nil
	})
	if err != nil {
		return 0, 0, err
	}
	return int(ipVal), int(acctVal), nil
}

func (s *RedisStore) IsLoginLocked(ctx context.Context, ip, account string) (bool, error) {
	ipKey := adminLockKeyPrefix + ip
	acctKey := adminAcctLockPrefix + account

	var locked bool
	_, err := s.cb.Execute(func() (any, error) {
		pipe := s.rdb.Pipeline()
		ipLock := pipe.Exists(ctx, ipKey)
		acctLock := pipe.Exists(ctx, acctKey)
		_, execErr := pipe.Exec(ctx)
		if execErr != nil {
			return nil, fmt.Errorf("check login lock: %w", execErr)
		}
		ipLocked, _ := ipLock.Result()
		acctLocked, _ := acctLock.Result()
		locked = ipLocked > 0 || acctLocked > 0
		return nil, nil
	})
	if err != nil {
		return false, err
	}
	return locked, nil
}

func (s *RedisStore) SetLoginLock(ctx context.Context, ip, account string, ttl time.Duration) error {
	ipKey := adminLockKeyPrefix + ip
	acctKey := adminAcctLockPrefix + account

	_, err := s.cb.Execute(func() (any, error) {
		pipe := s.rdb.Pipeline()
		pipe.Set(ctx, ipKey, "1", ttl)
		pipe.Set(ctx, acctKey, "1", ttl)
		_, execErr := pipe.Exec(ctx)
		if execErr != nil {
			return nil, fmt.Errorf("set login lock: %w", execErr)
		}
		return nil, nil
	})
	return err
}

func (s *RedisStore) ResetFailedLogin(ctx context.Context, ip, account string) error {
	ipFailKey := adminFailKeyPrefix + ip
	ipLockKey := adminLockKeyPrefix + ip
	acctFailKey := adminAcctFailPrefix + account
	acctLockKey := adminAcctLockPrefix + account

	_, err := s.cb.Execute(func() (any, error) {
		pipe := s.rdb.Pipeline()
		pipe.Del(ctx, ipFailKey, ipLockKey)
		pipe.Del(ctx, acctFailKey, acctLockKey)
		_, execErr := pipe.Exec(ctx)
		if execErr != nil {
			return nil, fmt.Errorf("reset failed login: %w", execErr)
		}
		return nil, nil
	})
	return err
}

func (s *RedisStore) AddAdminJTI(ctx context.Context, jti string, ttl time.Duration) error {
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

func (s *RedisStore) RemoveAdminJTI(ctx context.Context, jti string) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.SRem(ctx, adminActiveJTISetKey, jti).Err(); err != nil {
			return nil, fmt.Errorf("remove admin jti: %w", err)
		}
		return nil, nil
	})
	return err
}

func (s *RedisStore) GetAllAdminJTIs(ctx context.Context) ([]string, error) {
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