package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RoomRegistryInfo is room ownership metadata stored in Redis (ADR-005).
type RoomRegistryInfo struct {
	Code      string `json:"code"`
	Instance  string `json:"instance"`
	Address   string `json:"address"`
	CreatedAt int64  `json:"created_at"`
}

// GetRoomRegistry loads room registry metadata from Redis.
func (s *RedisStore) GetRoomRegistry(ctx context.Context, code string) (*RoomRegistryInfo, error) {
	raw, err := s.rdb.Get(ctx, roomInfoKey(code)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get room registry: %w", err)
	}
	var info RoomRegistryInfo
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
