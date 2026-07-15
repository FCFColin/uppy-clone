package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const lobbyListCachePrefix = "lobby:list:"
const lobbyCheckCachePrefix = "lobby:check:"

// LobbyReadCacheTTL is the TTL for cached lobby list and room-check responses.
const LobbyReadCacheTTL = 30 * time.Second

func lobbyListCacheKey(limit int, cursor string) string {
	return fmt.Sprintf("%s%d:%s", lobbyListCachePrefix, limit, cursor)
}

func lobbyCheckCacheKey(code string) string {
	return lobbyCheckCachePrefix + code
}

// LobbyReadCacheStore handles ADR-006 read-through cache for lobby list and
// room-check responses in Redis.
type LobbyReadCacheStore struct {
	baseRedisStore
}

// NewLobbyReadCacheStore creates a LobbyReadCacheStore.
func NewLobbyReadCacheStore(rdb *redis.Client, deps ...Deps) *LobbyReadCacheStore {
	d := depsOrZero(deps...)
	return &LobbyReadCacheStore{baseRedisStore: newBaseRedisStore(rdb, d)}
}

// GetCachedLobbyList returns cached lobby list JSON if present.
func (s *LobbyReadCacheStore) GetCachedLobbyList(ctx context.Context, limit int, cursor string) ([]byte, bool, error) {
	var data []byte
	var found bool
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyListCacheKey(limit, cursor)
		val, err := s.rdb.Get(ctx, key).Bytes()
		if err == redis.Nil {
			data, found = nil, false
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get lobby list cache: %w", err)
		}
		data, found = val, true
		return nil, nil
	})
	if err != nil {
		return nil, false, err
	}
	return data, found, nil
}

// SetCachedLobbyList stores lobby list JSON with ADR-006 TTL.
func (s *LobbyReadCacheStore) SetCachedLobbyList(ctx context.Context, limit int, cursor string, data []byte) error {
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyListCacheKey(limit, cursor)
		if err := s.rdb.Set(ctx, key, data, LobbyReadCacheTTL).Err(); err != nil {
			return nil, fmt.Errorf("set lobby list cache: %w", err)
		}
		return nil, nil
	})
	return err
}

// GetCachedRoomCheck returns cached room check JSON if present.
func (s *LobbyReadCacheStore) GetCachedRoomCheck(ctx context.Context, code string) ([]byte, bool, error) {
	var data []byte
	var found bool
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyCheckCacheKey(code)
		val, err := s.rdb.Get(ctx, key).Bytes()
		if err == redis.Nil {
			data, found = nil, false
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get room check cache: %w", err)
		}
		data, found = val, true
		return nil, nil
	})
	if err != nil {
		return nil, false, err
	}
	return data, found, nil
}

// SetCachedRoomCheck stores room check JSON with ADR-006 TTL.
func (s *LobbyReadCacheStore) SetCachedRoomCheck(ctx context.Context, code string, data []byte) error {
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyCheckCacheKey(code)
		if err := s.rdb.Set(ctx, key, data, LobbyReadCacheTTL).Err(); err != nil {
			return nil, fmt.Errorf("set room check cache: %w", err)
		}
		return nil, nil
	})
	return err
}

// InvalidateLobbyListCaches removes all paginated lobby list cache entries.
func (s *LobbyReadCacheStore) InvalidateLobbyListCaches(ctx context.Context) error {
	_, err := s.cb.Execute(func() (any, error) {
		var cursor uint64
		for {
			keys, next, err := s.rdb.Scan(ctx, cursor, lobbyListCachePrefix+"*", 100).Result()
			if err != nil {
				return nil, fmt.Errorf("scan lobby list cache keys: %w", err)
			}
			if len(keys) > 0 {
				if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
					return nil, fmt.Errorf("delete lobby list cache keys: %w", err)
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		return nil, nil
	})
	return err
}

// InvalidateRoomCheck removes the check cache for a single room.
func (s *LobbyReadCacheStore) InvalidateRoomCheck(ctx context.Context, code string) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.Del(ctx, lobbyCheckCacheKey(code)).Err(); err != nil {
			return nil, fmt.Errorf("delete room check cache: %w", err)
		}
		return nil, nil
	})
	return err
}
