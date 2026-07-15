package game

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
)

// ─── Instance Address & Cache Invalidation ───────────────────────────

// instanceAddress returns the network address other instances should use to
// reach this instance. It is recorded in the Redis room registry by
// registerRoomInRedis. The owner-reverse-proxy routing layer that originally
// consumed this address was never implemented (see ADR-005); the value is
// still stored for diagnostic purposes and future use.
func instanceAddress() string {
	if addr := os.Getenv("INSTANCE_ADDR"); addr != "" {
		return addr
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}
	return fmt.Sprintf("127.0.0.1:%s", port)
}

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
		h.logger.Warn("room registry lookup failed", "code", code, "error", err)
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
			Address:   instanceAddress(),
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

// ─── Room Removal ───────────────────────────────────────────────────

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
			h.logger.Info(logMsg, "code", code)
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
			Before:    map[string]interface{}{"code": code},
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

// ─── Room Restore & Materialization ─────────────────────────────────

// RestoreRooms 从 PostgreSQL 加载本实例应拥有的活跃房间（启动时调用）。
func (h *Hub) RestoreRooms() error {
	if h.store == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGRequestTimeout)
	defer cancel()

	cursor := ""
	const pageSize = 100
	restored := 0

	for {
		result, err := h.store.LoadAllActiveLobbies(ctx, pageSize, cursor)
		if err != nil {
			return err
		}
		if len(result.Lobbies) == 0 {
			break
		}

		var toFinalize []string
		h.mu.Lock()
		for _, ls := range result.Lobbies {
			if _, ok := h.rooms[ls.Code]; ok {
				continue
			}
			if !h.shouldLocalMaterializeRoom(ctx, ls.Code) {
				continue
			}
			room, err := h.deserializeAndMaterialize(ls.Code, []byte(ls.State))
			if err != nil {
				h.logger.Error("deserialize lobby state on restore", "code", ls.Code, "error", err)
				continue
			}
			h.rooms[ls.Code] = room
			toFinalize = append(toFinalize, ls.Code)
			restored++
			h.logger.Info("restored room", "code", ls.Code, "phase", room.state.Phase)
		}
		h.mu.Unlock()

		for _, code := range toFinalize {
			h.finalizeMaterializedRoom(code)
		}

		if len(result.Lobbies) < pageSize {
			break
		}
		cursor = result.Lobbies[len(result.Lobbies)-1].Code
	}

	h.logger.Info("rooms restored", "count", restored)
	return nil
}

func (h *Hub) materializeRoom(code string, state *domain.GameState) *Room {
	room := NewRoom(code, h, h.store, h.timeouts, h.maxPlayersPerRoom)
	room.state = state
	room.rng = newSeededRNG(state.RNGSeed)
	for _, p := range state.Players {
		room.usedNames[p.Nickname] = true
	}
	return room
}

func (h *Hub) deserializeAndMaterialize(code string, stateJSON []byte) (*Room, error) {
	state, err := DeserializeState(stateJSON)
	if err != nil {
		return nil, err
	}
	return h.materializeRoom(code, state), nil
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
		h.logger.Error("deserialize lobby state", "code", code, "error", err)
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

// ─── Cleanup Loop ────────────────────────────────────────────────────

// CleanupLoop 定期清理空房间
func (h *Hub) CleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(h.cleanupLoopInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupOnce()
		}
	}
}

func (h *Hub) cleanupOnce() {
	type roomEntry struct {
		code string
		room *Room
	}

	h.mu.RLock()
	entries := make([]roomEntry, 0, len(h.rooms))
	for code, room := range h.rooms {
		entries = append(entries, roomEntry{code: code, room: room})
	}
	h.mu.RUnlock()

	now := time.Now().UnixMilli()
	var toCleanup []string
	for _, entry := range entries {
		if shouldCleanupRoom(entry.room, now) {
			toCleanup = append(toCleanup, entry.code)
		}
	}

	h.removeRooms(toCleanup, removeRoomOptions{
		pgDelete: true,
		cache:    true,
		logMsg:   "cleaned up empty room",
	})
}

func shouldCleanupRoom(room *Room, now int64) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()

	phase := room.state.Phase
	playerCount := len(room.state.Players)
	hasConnections := len(room.connections) > 0

	if phase == domain.PhaseWaiting && !hasConnections {
		return true
	}
	if playerCount == 0 && !hasConnections {
		return true
	}
	if !hasConnections && playerCount > 0 {
		return allPlayersDisconnectedExpired(room.state.Players, now)
	}
	return false
}

func allPlayersDisconnectedExpired(players map[string]*domain.PlayerState, now int64) bool {
	for _, p := range players {
		if !p.Disconnected || p.DisconnectedAt == nil {
			return false
		}
		if now-*p.DisconnectedAt <= domain.ReconnectGraceMs {
			return false
		}
	}
	return true
}

func (h *Hub) cleanupLoopInterval() time.Duration {
	if h.cleanupInterval > 0 {
		return h.cleanupInterval
	}
	return config.CleanupInterval
}
