package game

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
)

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

// ListLobbiesCached uses Redis read-through cache per ADR-006 when available.
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

func (h *Hub) RemoveRoom(ctx context.Context, code string) {
	h.removeSingleRoom(ctx, code, removeRoomOptions{
		audit:    true,
		pgDelete: true,
		cache:    true,
		logMsg:   "room removed",
	})
}

// CloseAllRooms closes all active rooms, ensuring state is persisted.
func (h *Hub) CloseAllRooms() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for code, room := range h.rooms {
		room.Close()
		delete(h.rooms, code)
		h.logger.Info("room closed on shutdown", codeKey, code)
	}
	metrics.ActiveRooms.Set(0)
}

func (h *Hub) RoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}

func (h *Hub) PlayerCount() int {
	h.mu.RLock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, room := range h.rooms {
		rooms = append(rooms, room)
	}
	h.mu.RUnlock()
	total := 0
	for _, room := range rooms {
		room.mu.RLock()
		total += len(room.state.Players)
		room.mu.RUnlock()
	}
	return total
}

func (h *Hub) PhaseCounts() map[string]int {
	h.mu.RLock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, room := range h.rooms {
		rooms = append(rooms, room)
	}
	h.mu.RUnlock()
	counts := make(map[string]int)
	for _, room := range rooms {
		room.mu.RLock()
		phase := string(room.state.Phase)
		room.mu.RUnlock()
		counts[phase]++
	}
	return counts
}

func roomJoinable(room *Room, maxPlayers int) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return room.state.Phase == domain.PhaseWaiting &&
		len(room.state.Players) < maxPlayers &&
		len(room.connections) < maxPlayers
}

func (h *Hub) MatchRoom(ctx context.Context) (string, error) {
	for _, code := range h.joinableRoomCodes() {
		h.mu.RLock()
		room := h.rooms[code]
		h.mu.RUnlock()
		if room != nil && roomJoinable(room, h.maxPlayersPerRoom) {
			return code, nil
		}
	}

	return h.CreateRoom(ctx)
}

func (h *Hub) joinableRoomCodes() []string {
	h.mu.RLock()
	rooms := make([]*Room, 0, len(h.rooms))
	codes := make([]string, 0, len(h.rooms))
	for code, room := range h.rooms {
		rooms = append(rooms, room)
		codes = append(codes, code)
	}
	h.mu.RUnlock()
	results := make([]string, 0, len(rooms))
	for i, room := range rooms {
		if roomJoinable(room, h.maxPlayersPerRoom) {
			results = append(results, codes[i])
		}
	}
	return results
}

type removeRoomOptions struct {
	audit    bool
	pgDelete bool
	cache    bool
	logMsg   string
}

func (h *Hub) removeRoomFromMemory(code string, logMsg string) *Room {
	var room *Room
	if r, ok := h.rooms[code]; ok {
		room = r
		delete(h.rooms, code)
		metrics.ActiveRooms.Set(float64(len(h.rooms)))
		if logMsg != "" {
			h.logger.Info(logMsg, codeKey, code)
		}
	}
	return room
}

func (h *Hub) finalizeRoomRemoval(ctx context.Context, code string, room *Room, opts removeRoomOptions) {
	if room != nil {
		room.Close()
	}

	if opts.pgDelete && h.store != nil {
		pctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGQueryTimeout)
		defer cancel()
		_ = h.store.DeleteLobbyState(pctx, code)
	}

	h.unregisterRoomFromRedis(code)

	if opts.cache {
		h.invalidateLobbyReadCaches(code)
	}

	if opts.audit {
		audit.Log(ctx, audit.AuditEntry{
			Action:    "room.delete",
			ActorType: audit.ActorTypeSystem,
			ActorID:   "system",
			Resource:  "room/" + code,
			Before:    map[string]interface{}{codeKey: code},
		})
	}
}

func (h *Hub) removeRooms(codes []string, opts removeRoomOptions) {
	if len(codes) == 0 {
		return
	}

	type codeRoom struct {
		code string
		room *Room
	}
	var batch []codeRoom

	h.mu.Lock()
	for _, code := range codes {
		if room := h.removeRoomFromMemory(code, opts.logMsg); room != nil {
			batch = append(batch, codeRoom{code: code, room: room})
		}
	}
	h.mu.Unlock()

	for _, item := range batch {
		h.finalizeRoomRemoval(context.Background(), item.code, item.room, opts)
	}
}

func (h *Hub) removeSingleRoom(ctx context.Context, code string, opts removeRoomOptions) {
	var room *Room
	h.mu.Lock()
	room = h.removeRoomFromMemory(code, opts.logMsg)
	h.mu.Unlock()
	h.finalizeRoomRemoval(ctx, code, room, opts)
}

func (h *Hub) registerRoomLocked(code string, room *Room) *Room {
	if existing, ok := h.rooms[code]; ok {
		return existing
	}
	h.rooms[code] = room
	return room
}

// loadOrMaterializeRoom 从 PG 加载房间并注册到内存（I/O 在锁外执行）。
func (h *Hub) loadOrMaterializeRoom(code string) *Room {
	if h.store == nil {
		return nil
	}

	// I/O operations outside lock
	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGQueryTimeout)
	defer cancel()

	if !h.shouldLocalMaterializeRoom(ctx, code) {
		return nil
	}

	ls, err := h.store.LoadLobbyState(ctx, code)
	if err != nil || ls == nil {
		return nil
	}

	room, err := h.deserializeAndMaterialize(code, []byte(ls.State))
	if err != nil {
		h.logger.Error("deserialize lobby state", codeKey, code, "error", err)
		return nil
	}

	// Register under lock
	h.mu.Lock()
	room = h.registerRoomLocked(code, room)
	h.mu.Unlock()

	if room != nil {
		h.finalizeMaterializedRoom(code)
	}
	return room
}
