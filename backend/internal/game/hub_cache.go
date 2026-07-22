package game

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
)

// ─── Cache Invalidation ──────────────────────────────────────────────

// invalidateLobbyReadCaches clears ADR-006 read caches after room mutations.
func (h *Hub) invalidateLobbyReadCaches(code string) {
	if h.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.RedisConnectTimeout)
	defer cancel()
	_ = h.cache.InvalidateLobbyListCaches(ctx)
	if code != "" {
		_ = h.cache.InvalidateRoomCheck(ctx, code)
	}
}

// ─── Redis Room Registry ─────────────────────────────────────────────

const roomRegistryTTL = 24 * time.Hour

func (h *Hub) shouldLocalMaterializeRoom(ctx context.Context, code string) bool {
	if h.cache == nil {
		return true
	}
	info, err := h.cache.GetRoomRegistry(ctx, code)
	if err != nil {
		h.logger.Warn("room registry lookup failed", codeKey, code, "error", err)
		// game-026: fail-closed — on Redis/registry error, do NOT materialize room locally.
		return false
	}
	if info == nil || info.Instance == "" {
		return true
	}
	return info.Instance == h.instanceID
}

func (h *Hub) finalizeMaterializedRoom(code string) {
	h.registerRoomInRedis(code)
}

func (h *Hub) cacheOp(fn func(context.Context) error) {
	if h.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.RedisConnectTimeout)
	defer cancel()
	_ = fn(ctx)
}

func (h *Hub) registerRoomInRedis(code string) {
	h.cacheOp(func(ctx context.Context) error {
		data, _ := json.Marshal(domain.RoomRegistryInfo{
			Code:      code,
			Instance:  h.instanceID,
			CreatedAt: time.Now().UnixMilli(),
		})
		return h.cache.RegisterRoom(ctx, code, data, roomRegistryTTL)
	})
}

func (h *Hub) unregisterRoomFromRedis(code string) {
	h.cacheOp(func(ctx context.Context) error {
		return h.cache.UnregisterRoom(ctx, code)
	})
}

// ─── Read-Through Cache (ADR-006) ────────────────────────────────────

// ListLobbiesCached returns active lobbies with cursor-based pagination.
// Uses Redis read-through cache per ADR-006 when available.
func (h *Hub) ListLobbiesCached(ctx context.Context, limit int, cursor string) (*domain.LobbyListResult, error) {
	if h.store == nil {
		return nil, fmt.Errorf("store not available")
	}

	if h.cache != nil {
		return readThroughCache(ctx,
			func(ctx context.Context) ([]byte, bool, error) {
				return h.cache.GetCachedLobbyList(ctx, limit, cursor)
			},
			func(ctx context.Context, data []byte) error {
				return h.cache.SetCachedLobbyList(ctx, limit, cursor, data)
			},
			func(ctx context.Context) (*domain.LobbyListResult, error) {
				return h.store.LoadAllActiveLobbies(ctx, limit, cursor)
			},
		)
	}

	return h.store.LoadAllActiveLobbies(ctx, limit, cursor)
}

// roomCheckNegativeMarker is cached for non-existent rooms (game-023).
var roomCheckNegativeMarker = []byte("null")

// CheckRoomCached checks room existence with Redis read-through cache per ADR-006.
// game-023: Includes negative caching — when a room doesn't exist, the negative
// result is cached briefly to avoid repeated DB queries for non-existent codes.
func (h *Hub) CheckRoomCached(ctx context.Context, code string) (*RoomInfo, error) {
	if h.cache != nil {
		cached, ok, err := h.cache.GetCachedRoomCheck(ctx, code)
		if err != nil {
			return nil, err
		}
		if ok {
			// game-023: Check for negative cache marker.
			if bytes.Equal(cached, roomCheckNegativeMarker) {
				return nil, nil
			}
			var info RoomInfo
			if json.Unmarshal(cached, &info) == nil {
				return &info, nil
			}
		}
	}

	info, err := h.CheckRoom(code)
	if err != nil || info == nil {
		// game-023: Cache negative result briefly to avoid repeated DB lookups.
		if err == nil && h.cache != nil {
			_ = h.cache.SetCachedRoomCheck(ctx, code, roomCheckNegativeMarker)
		}
		return info, err
	}

	if h.cache != nil {
		if data, err := json.Marshal(info); err == nil {
			_ = h.cache.SetCachedRoomCheck(ctx, code, data)
		}
	}
	return info, err
}

func readThroughCache[T any](
	ctx context.Context,
	get func(context.Context) ([]byte, bool, error),
	set func(context.Context, []byte) error,
	load func(context.Context) (T, error),
) (T, error) {
	var zero T
	if cached, ok, err := get(ctx); ok && err == nil {
		var result T
		if json.Unmarshal(cached, &result) == nil {
			return result, nil
		}
	}
	result, err := load(ctx)
	if err != nil {
		return zero, err
	}
	if data, err := json.Marshal(result); err == nil {
		_ = set(ctx, data)
	}
	return result, nil
}
