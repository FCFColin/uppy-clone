package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/domain"
)

// GetRoomRegistry loads room registry metadata from Redis.
func (s *RedisStore) GetRoomRegistry(ctx context.Context, code string) (*domain.RoomRegistryInfo, error) {
	var info *domain.RoomRegistryInfo
	_, err := s.cb.Execute(func() (any, error) {
		raw, err := s.rdb.Get(ctx, roomInfoKey(code)).Bytes()
		if err == redis.Nil {
			info = nil
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get room registry: %w", err)
		}
		var v domain.RoomRegistryInfo
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("unmarshal room registry: %w", err)
		}
		info = &v
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return info, nil
}

// RegisterRoom stores room ownership metadata for multi-instance routing.
func (s *RedisStore) RegisterRoom(ctx context.Context, code string, data []byte, ttl time.Duration) error {
	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if setErr := s.rdb.Set(ctx, key, data, ttl).Err(); setErr != nil {
			return nil, fmt.Errorf("register room: %w", setErr)
		}
		return nil, nil
	})
	return err
}

// UnregisterRoom removes room ownership metadata from Redis.
func (s *RedisStore) UnregisterRoom(ctx context.Context, code string) error {
	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if delErr := s.rdb.Del(ctx, key).Err(); delErr != nil {
			return nil, fmt.Errorf("unregister room: %w", delErr)
		}
		return nil, nil
	})
	return err
}

// RenewRoomRegistry extends room registry TTL (owner lease heartbeat).
func (s *RedisStore) RenewRoomRegistry(ctx context.Context, code string, ttl time.Duration) error {
	_, err := s.cb.Execute(func() (any, error) {
		key := roomInfoKey(code)
		if err := s.rdb.Expire(ctx, key, ttl).Err(); err != nil {
			return nil, fmt.Errorf("renew room registry ttl: %w", err)
		}
		return nil, nil
	})
	return err
}