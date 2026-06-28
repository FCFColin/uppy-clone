package game

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/uppy-clone/backend/internal/store"
)

// RoomRoute describes how a room connection should be routed (ADR-005).
type RoomRoute int

const (
	// RouteLocal serves the room on this instance.
	RouteLocal RoomRoute = iota
	// RouteProxy forwards the connection to the room owner instance.
	RouteProxy
)

// RoomRouteDecision is the result of ResolveRoom.
type RoomRouteDecision struct {
	Route   RoomRoute
	Owner   string
	Address string
}

func instanceAddress() string {
	if addr := os.Getenv("INSTANCE_ADDR"); addr != "" {
		return addr
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("127.0.0.1:%s", port)
}

// ResolveRoom determines whether the room should be served locally or proxied to the owner instance.
func (h *Hub) ResolveRoom(ctx context.Context, code string) (RoomRouteDecision, error) {
	decision := RoomRouteDecision{Route: RouteLocal, Owner: h.instanceID, Address: instanceAddress()}
	if h.redis == nil {
		return decision, nil
	}

	info, err := h.redis.GetRoomRegistry(ctx, code)
	if err != nil {
		return decision, err
	}
	if info == nil || info.Instance == "" {
		return decision, nil
	}

	decision.Owner = info.Instance
	decision.Address = info.Address
	if info.Instance != h.instanceID {
		decision.Route = RouteProxy
	}
	return decision, nil
}

// invalidateLobbyReadCaches clears ADR-006 read caches after room mutations.
func (h *Hub) invalidateLobbyReadCaches(code string) {
	if h.redis == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.RedisConnectTimeout)
	defer cancel()
	_ = h.redis.InvalidateLobbyListCaches(ctx)
	if code != "" {
		_ = h.redis.InvalidateRoomCheck(ctx, code)
	}
}

// ListLobbiesCached returns active lobbies with cursor-based pagination.
// Uses Redis read-through cache per ADR-006 when available.
func (h *Hub) ListLobbiesCached(ctx context.Context, limit int, cursor string) (*store.LobbyListResult, error) {
	if h.store == nil {
		return nil, fmt.Errorf("store not available")
	}

	if h.redis != nil {
		return readThroughCache(ctx,
			func(ctx context.Context) ([]byte, bool, error) {
				return h.redis.GetCachedLobbyList(ctx, limit, cursor)
			},
			func(ctx context.Context, data []byte) error {
				return h.redis.SetCachedLobbyList(ctx, limit, cursor, data)
			},
			func(ctx context.Context) (*store.LobbyListResult, error) {
				return h.store.LoadAllActiveLobbies(ctx, limit, cursor)
			},
		)
	}

	return h.store.LoadAllActiveLobbies(ctx, limit, cursor)
}

// CheckRoomCached checks room existence with Redis read-through cache per ADR-006.
func (h *Hub) CheckRoomCached(ctx context.Context, code string) (*RoomInfo, error) {
	if h.redis != nil {
		if cached, ok, err := h.redis.GetCachedRoomCheck(ctx, code); ok && err == nil {
			var info RoomInfo
			if json.Unmarshal(cached, &info) == nil {
				return &info, nil
			}
		}
	}

	info, err := h.CheckRoom(code)
	if err != nil || info == nil {
		return info, err
	}

	if h.redis != nil {
		if data, err := json.Marshal(info); err == nil {
			_ = h.redis.SetCachedRoomCheck(ctx, code, data)
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
