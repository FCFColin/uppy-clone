package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RoomRegistryStore handles room registry operations in Redis.
type RoomRegistryStore struct {
	baseRedisStore
}

// NewRoomRegistryStore creates a RoomRegistryStore.
func NewRoomRegistryStore(rdb *redis.Client) *RoomRegistryStore {
	return &RoomRegistryStore{baseRedisStore: newBaseRedisStore(rdb)}
}

func (s *RoomRegistryStore) TryClaimRoomRegistry(ctx context.Context, code string, data []byte, instanceID string, ttl time.Duration) (bool, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "room_registry.TryClaimRoomRegistry",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SETNX"),
		),
	)
	defer span.End()

	key := roomInfoKey(code)
	var claimed bool
	_, err := s.cb.Execute(func() (any, error) {
		ok, setErr := s.rdb.SetNX(ctx, key, data, ttl).Result()
		if setErr != nil {
			return nil, fmt.Errorf("try claim room registry: %w", setErr)
		}
		claimed = ok
		return nil, nil
	})
	return claimed, err
}

func (s *RoomRegistryStore) RegisterRoom(ctx context.Context, code string, data []byte, ttl time.Duration) error {
	ctx, span := telemetry.Tracer().Start(ctx, "room_registry.RegisterRoom",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SET"),
		),
	)
	defer span.End()

	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if setErr := s.rdb.Set(ctx, key, data, ttl).Err(); setErr != nil {
			return nil, fmt.Errorf("register room: %w", setErr)
		}
		return nil, nil
	})
	return err
}

func (s *RoomRegistryStore) UnregisterRoom(ctx context.Context, code string) error {
	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if delErr := s.rdb.Del(ctx, key).Err(); delErr != nil {
			return nil, fmt.Errorf("unregister room: %w", delErr)
		}
		return nil, nil
	})
	return err
}

func (s *RoomRegistryStore) ListActiveRooms(ctx context.Context) ([]string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "room_registry.ListActiveRooms",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SCAN"),
		),
	)
	defer span.End()

	pattern := "room:*"
	var keys []string
	_, err := s.cb.Execute(func() (any, error) {
		var cursor uint64
		for {
			var batch []string
			var scanErr error
			batch, cursor, scanErr = s.rdb.Scan(ctx, cursor, pattern, 100).Result()
			if scanErr != nil {
				return nil, fmt.Errorf("list active rooms: %w", scanErr)
			}
			keys = append(keys, batch...)
			if cursor == 0 {
				break
			}
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *RoomRegistryStore) GetRoomRegistry(ctx context.Context, code string) (*domain.RoomRegistryInfo, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "room_registry.GetRoomRegistry",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "GET"),
		),
	)
	defer span.End()

	raw, err := s.rdb.Get(ctx, roomInfoKey(code)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get room registry: %w", err)
	}
	info, unmarshalErr := domain.UnmarshalRoomRegistryInfo(raw)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return info, nil
}

func (s *RoomRegistryStore) RenewRoomRegistry(ctx context.Context, code string, ttl time.Duration) error {
	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if expireErr := s.rdb.Expire(ctx, key, ttl).Err(); expireErr != nil {
			return nil, fmt.Errorf("renew room registry: %w", expireErr)
		}
		return nil, nil
	})
	return err
}
