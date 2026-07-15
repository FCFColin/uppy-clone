package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/domain"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RoomRegistryStore handles room registry operations in Redis.
type RoomRegistryStore struct {
	baseRedisStore
}

// NewRoomRegistryStore creates a RoomRegistryStore.
func NewRoomRegistryStore(rdb *redis.Client, deps ...Deps) *RoomRegistryStore {
	d := depsOrZero(deps...)
	return &RoomRegistryStore{baseRedisStore: newBaseRedisStore(rdb, d)}
}

// TryClaimRoomRegistry atomically claims a room registry entry using SETNX.
func (s *RoomRegistryStore) TryClaimRoomRegistry(ctx context.Context, code string, data []byte, _ string, ttl time.Duration) (bool, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "room_registry.TryClaimRoomRegistry",
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
		if ok {
			// Maintain SET index for O(1) listing (store-015).
			if sAddErr := s.rdb.SAdd(ctx, roomIndexKey(), code).Err(); sAddErr != nil {
				return nil, fmt.Errorf("try claim room index: %w", sAddErr)
			}
		}
		return nil, nil
	})
	return claimed, err
}

// RegisterRoom stores room info in Redis with a TTL and adds it to the room index.
func (s *RoomRegistryStore) RegisterRoom(ctx context.Context, code string, data []byte, ttl time.Duration) error {
	ctx, span := s.deps.Tracer.Start(ctx, "room_registry.RegisterRoom",
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
		// Maintain a SET index for O(1) listing instead of SCAN (store-015).
		if sAddErr := s.rdb.SAdd(ctx, roomIndexKey(), code).Err(); sAddErr != nil {
			return nil, fmt.Errorf("register room index: %w", sAddErr)
		}
		return nil, nil
	})
	return err
}

// UnregisterRoom removes a room and its index entry from Redis.
func (s *RoomRegistryStore) UnregisterRoom(ctx context.Context, code string) error {
	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if delErr := s.rdb.Del(ctx, key).Err(); delErr != nil {
			return nil, fmt.Errorf("unregister room: %w", delErr)
		}
		// Remove from the SET index (store-015).
		if sRemErr := s.rdb.SRem(ctx, roomIndexKey(), code).Err(); sRemErr != nil {
			return nil, fmt.Errorf("unregister room index: %w", sRemErr)
		}
		return nil, nil
	})
	return err
}

// ListActiveRooms returns all room codes in the room index set.
func (s *RoomRegistryStore) ListActiveRooms(ctx context.Context) ([]string, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "room_registry.ListActiveRooms",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SMEMBERS"),
		),
	)
	defer span.End()

	var keys []string
	_, err := s.cb.Execute(func() (any, error) {
		members, sErr := s.rdb.SMembers(ctx, roomIndexKey()).Result()
		if sErr != nil {
			return nil, fmt.Errorf("list active rooms: %w", sErr)
		}
		keys = members
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// GetRoomRegistry retrieves room registry info by code.
func (s *RoomRegistryStore) GetRoomRegistry(ctx context.Context, code string) (*domain.RoomRegistryInfo, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "room_registry.GetRoomRegistry",
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

// RenewRoomRegistry extends the TTL of an existing room registry entry.
func (s *RoomRegistryStore) RenewRoomRegistry(ctx context.Context, code string, ttl time.Duration) error {
	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		// store-030: Check the boolean return value of Expire. If the key doesn't
		// exist (already expired or was deleted), Expire returns false with no error.
		// Treating this as success would silently lose the renewal — callers expect
		// RenewRoomRegistry to extend the TTL of an existing room.
		ok, expireErr := s.rdb.Expire(ctx, key, ttl).Result()
		if expireErr != nil {
			return nil, fmt.Errorf("renew room registry: %w", expireErr)
		}
		if !ok {
			return nil, fmt.Errorf("renew room registry: room %s not found (key expired or deleted)", code)
		}
		return nil, nil
	})
	return err
}
