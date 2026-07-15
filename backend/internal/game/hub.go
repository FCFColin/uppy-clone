package game

import (
	"context"
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
	codeGen           *RoomCodeGenerator
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
		codeGen:           NewRoomCodeGenerator(time.Now().UnixNano()),
		wsLimiter:         NewWSLimiter(maxWSConnections),
	}
	return h
}

// CreateRoom creates a new room and returns its code.
func (h *Hub) CreateRoom(ctx context.Context) (string, error) {
	h.mu.Lock()

	var code string
	for i := 0; i < 10; i++ {
		code = h.codeGen.generateRoomCode()
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
		h.logger.Error("generated invalid room code", "code", code, "error", err)
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
		After:     map[string]interface{}{"code": code, "max_players": h.maxPlayersPerRoom},
	})

	h.logger.Info("room created", "code", code)
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
		h.logger.Info("room closed on shutdown", "code", code)
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

// ─── Room Code Generator ──────────────────────────────────────────────

// RoomCodeGenerator generates unique room codes. Extracted from Hub to
// isolate the RNG state and hook-override logic from the room registry.
type RoomCodeGenerator struct {
	rng   RNGSource
	mu    sync.Mutex
	genFn func() string
}

// NewRoomCodeGenerator creates a RoomCodeGenerator seeded with the given value.
func NewRoomCodeGenerator(seed int64) *RoomCodeGenerator {
	g := &RoomCodeGenerator{
		rng: newSeededRNG(seed),
	}
	g.genFn = func() string {
		return GenerateRoomCode(g.rng)
	}
	return g
}

// SetGenerateRoomCodeHook overrides room code generation and returns a
// restore func. Tests use this to force deterministic room codes.
func (g *RoomCodeGenerator) SetGenerateRoomCodeHook(fn func() string) (restore func()) {
	prev := g.genFn
	g.genFn = fn
	return func() { g.genFn = prev }
}

// generateRoomCode returns a room code, using the hook if set, otherwise
// the default RNG-based generator.
func (g *RoomCodeGenerator) generateRoomCode() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.genFn != nil {
		return g.genFn()
	}
	return GenerateRoomCode(g.rng)
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
