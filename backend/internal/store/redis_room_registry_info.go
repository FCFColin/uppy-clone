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
	raw, err := s.rdb.Get(ctx, roomInfoKey(code)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get room registry: %w", err)
	}
	var info domain.RoomRegistryInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("unmarshal room registry: %w", err)
	}
	return &info, nil
}

// RenewRoomRegistry extends room registry TTL (owner lease heartbeat).
func (s *RedisStore) RenewRoomRegistry(ctx context.Context, code string, ttl time.Duration) error {
	key := roomInfoKey(code)
	if err := s.rdb.Expire(ctx, key, ttl).Err(); err != nil {
		return fmt.Errorf("renew room registry ttl: %w", err)
	}
	return nil
}
