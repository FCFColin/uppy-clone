package game

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
)

// unknownPlayerID is the fallback identifier used when hostname/instance
// lookup fails or when a WebSocket message type is unrecognized.
const unknownPlayerID = "unknown"

// codeKey is the JSON map key and structured-logging key used to identify a
// room code in audit entries, log records, and HTTP responses.
const codeKey = "code"

// defaultInstanceID returns the instance identifier: prefers the INSTANCE_ID
// environment variable, otherwise falls back to hostnameFunc(). Hub uses this
// for the Redis room registry (registerRoomInRedis) to record which instance
// owns a room.
func defaultInstanceID(hostnameFunc func() (string, error)) string {
	if id := os.Getenv("INSTANCE_ID"); id != "" {
		return id
	}
	hostname, err := hostnameFunc()
	if err != nil || hostname == "" {
		return unknownPlayerID
	}
	return hostname
}

// RoomInfo is a summary of a room.
type RoomInfo struct {
	Code        string
	Phase       string
	PlayerCount int
	CreatedAt   int64
}

// Hub manages all game rooms (analogous to the TS Registry).
type Hub struct {
	mu                sync.RWMutex
	rooms             map[string]*Room
	store             RoomRepository
	cache             CacheStore
	timeouts          config.TimeoutConfig
	logger            *slog.Logger
	maxPlayersPerRoom int
	instanceID        string
	observer          GameObserver // defaults to NoopGameObserver; set via SetObserver
	codeGen           *domain.RoomCodeGenerator
	wsLimiter         *WSLimiter
	cleanupInterval   time.Duration // 0 = use config.CleanupInterval; >0 for tests

}

// SetGenerateRoomCodeHook overrides room code generation for this Hub and returns a restore func.
func (h *Hub) SetGenerateRoomCodeHook(fn func() string) (restore func()) {
	return h.codeGen.SetGenerateRoomCodeHook(fn)
}

// NewHub creates a new Hub (room registry).
func NewHub(pgStore RoomRepository, cacheStore CacheStore, timeouts config.TimeoutConfig, maxWSConnections, maxPlayersPerRoom int) *Hub {
	if maxWSConnections <= 0 {
		maxWSConnections = config.MaxWSConnections
	}
	if maxPlayersPerRoom <= 0 {
		maxPlayersPerRoom = config.MaxPlayersPerRoom
	}
	h := &Hub{
		rooms:             make(map[string]*Room),
		store:             pgStore,
		cache:             cacheStore,
		timeouts:          timeouts,
		logger:            slog.Default().With("component", "hub"),
		maxPlayersPerRoom: maxPlayersPerRoom,
		instanceID:        defaultInstanceID(os.Hostname),
		codeGen:           domain.NewRoomCodeGenerator(time.Now().UnixNano()),
		wsLimiter:         NewWSLimiter(maxWSConnections),
	}
	return h
}

// CreateRoom creates a new room and returns its code.
func (h *Hub) CreateRoom(ctx context.Context) (string, error) {
	h.mu.Lock()

	var code string
	for i := 0; i < 10; i++ {
		code = h.codeGen.GenerateRoomCode()
		if _, exists := h.rooms[code]; !exists {
			break
		}
		code = ""
	}
	if code == "" {
		h.mu.Unlock()
		return "", ErrRoomCodeConflict
	}

	if _, err := domain.NewRoomCode(code); err != nil {
		h.mu.Unlock()
		h.logger.Error("generated invalid room code", codeKey, code, "error", err)
		return "", fmt.Errorf("invalid room code: %w", err)
	}

	room := NewRoom(code, h, h.store, h.timeouts, h.maxPlayersPerRoom)
	h.rooms[code] = room
	metrics.ActiveRooms.Set(float64(len(h.rooms)))
	h.mu.Unlock()

	h.registerRoomInRedis(code)
	h.invalidateLobbyReadCaches(code)

	audit.Log(ctx, audit.AuditEntry{
		Action:    "room.create",
		ActorType: audit.ActorTypeSystem,
		ActorID:   "system",
		Resource:  "room/" + code,
		After:     map[string]interface{}{codeKey: code, "max_players": h.maxPlayersPerRoom},
	})

	h.logger.Info("room created", codeKey, code)
	return code, nil
}

// GetRoom retrieves a room (loading from DB if not in memory) and returns it
// as a RoomHandle so callers depend only on the interface, not the concrete
// *Room type. Returns nil if the room cannot be found.
func (h *Hub) GetRoom(code string) RoomHandle {
	room := h.getRoom(code)
	if room == nil {
		return nil
	}
	return room
}

// getRoom is the internal accessor returning the concrete *Room for package
// callers (CheckRoom, MatchRoom, tests) that need fields/methods beyond the
// RoomHandle interface.
func (h *Hub) getRoom(code string) *Room {
	h.mu.RLock()
	room, ok := h.rooms[code]
	h.mu.RUnlock()
	if ok {
		return room
	}

	room = h.loadOrMaterializeRoom(code)
	return room
}

// RemoveRoom removes a room.
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

// CheckRoom 检查房间是否存在
func (h *Hub) CheckRoom(code string) (*RoomInfo, error) {
	room := h.getRoom(code)
	if room == nil {
		return nil, nil
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	return &RoomInfo{
		Code:        string(room.state.LobbyCode),
		Phase:       string(room.state.Phase),
		PlayerCount: len(room.state.Players),
		CreatedAt:   room.state.StartedAt,
	}, nil
}

// RoomCount 返回当前房间数量
func (h *Hub) RoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}

// PlayerCount 返回所有房间中的总玩家数
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

// PhaseCounts returns the number of rooms in each game phase.
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

// Timeouts returns the timeout configuration.
func (h *Hub) Timeouts() config.TimeoutConfig {
	return h.timeouts
}

// ErrRoomCodeConflict is returned when a room code collision occurs.
var ErrRoomCodeConflict = &roomCodeConflictError{}

type roomCodeConflictError struct{}

func (e *roomCodeConflictError) Error() string { return "room code conflict after 10 retries" }

// roomJoinable reports whether a waiting room can accept another player.
func roomJoinable(room *Room, maxPlayers int) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return room.state.Phase == domain.PhaseWaiting &&
		len(room.state.Players) < maxPlayers &&
		len(room.connections) < maxPlayers
}

// MatchRoom finds a joinable waiting room or creates a new one for quick play.
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

// ─── WebSocket Connection Limiter ────────────────────────────────────

// ErrWSConnectionLimit 全局 WebSocket 连接数已达上限
var ErrWSConnectionLimit = &wsConnectionLimitError{}

type wsConnectionLimitError struct{}

func (e *wsConnectionLimitError) Error() string { return "websocket connection limit reached" }

// WSLimiter manages the global WebSocket connection count and enforces the
// configured maximum (bulkhead pattern). Extracted from Hub to reduce the
// God-object surface: Hub delegates all WS connection accounting here.
type WSLimiter struct {
	maxWSConnections int
	wsConnCount      int64
}

// NewWSLimiter creates a WSLimiter with the given connection cap.
func NewWSLimiter(maxWSConnections int) *WSLimiter {
	return &WSLimiter{maxWSConnections: maxWSConnections}
}

// CanAcceptWSConnection 检查是否可以接受新的 WebSocket 连接
func (w *WSLimiter) CanAcceptWSConnection() bool {
	return atomic.LoadInt64(&w.wsConnCount) < int64(w.maxWSConnections)
}

// TryReserveWSConnection atomically reserves a WS slot before upgrade (avoids TOCTOU).
// Call DecrementWSConnection if upgrade/join fails after a successful reserve.
func (w *WSLimiter) TryReserveWSConnection() bool {
	for {
		current := atomic.LoadInt64(&w.wsConnCount)
		if current >= int64(w.maxWSConnections) {
			return false
		}
		if atomic.CompareAndSwapInt64(&w.wsConnCount, current, current+1) {
			metrics.WSConnections.Set(float64(current + 1))
			return true
		}
	}
}

// IncrementWSConnection increments the global WebSocket connection counter.
// game-016: Respects the maxWSConnections limit — will not increment beyond
// the configured cap. Use TryReserveWSConnection for the production path
// that returns a boolean. This function is primarily for tests.
func (w *WSLimiter) IncrementWSConnection() {
	for {
		current := atomic.LoadInt64(&w.wsConnCount)
		if current >= int64(w.maxWSConnections) {
			return
		}
		if atomic.CompareAndSwapInt64(&w.wsConnCount, current, current+1) {
			metrics.WSConnections.Set(float64(current + 1))
			return
		}
	}
}

// DecrementWSConnection decrements the global WebSocket connection counter.
func (w *WSLimiter) DecrementWSConnection() {
	count := atomic.AddInt64(&w.wsConnCount, -1)
	metrics.WSConnections.Set(float64(count))
}

// WSConnCount returns the current number of active WebSocket connections.
func (w *WSLimiter) WSConnCount() int64 {
	return atomic.LoadInt64(&w.wsConnCount)
}

// MaxWSConnections returns the configured global WebSocket connection limit.
func (w *WSLimiter) MaxWSConnections() int {
	return w.maxWSConnections
}

// ─── Hub delegating methods (preserve public API) ─────────────────────

// CanAcceptWSConnection delegates to WSLimiter.
func (h *Hub) CanAcceptWSConnection() bool {
	return h.wsLimiter.CanAcceptWSConnection()
}

// TryReserveWSConnection delegates to WSLimiter.
func (h *Hub) TryReserveWSConnection() bool {
	return h.wsLimiter.TryReserveWSConnection()
}

// IncrementWSConnection delegates to WSLimiter.
func (h *Hub) IncrementWSConnection() {
	h.wsLimiter.IncrementWSConnection()
}

// DecrementWSConnection delegates to WSLimiter.
func (h *Hub) DecrementWSConnection() {
	h.wsLimiter.DecrementWSConnection()
}

// WSConnCount delegates to WSLimiter.
func (h *Hub) WSConnCount() int64 {
	return h.wsLimiter.WSConnCount()
}

// MaxWSConnections delegates to WSLimiter.
func (h *Hub) MaxWSConnections() int {
	return h.wsLimiter.MaxWSConnections()
}

// MaxPlayersPerRoom returns the configured per-room player limit.
func (h *Hub) MaxPlayersPerRoom() int {
	return h.maxPlayersPerRoom
}

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
				h.logger.Error("deserialize lobby state on restore", codeKey, ls.Code, "error", err)
				continue
			}
			h.rooms[ls.Code] = room
			toFinalize = append(toFinalize, ls.Code)
			restored++
			h.logger.Info("restored room", codeKey, ls.Code, "phase", room.state.Phase)
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
