// Package game implements the multiplayer game engine.
package game

import (
	"context"
	"log/slog"
	"sync"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
)

// RoomInfo 房间摘要信息
type RoomInfo struct {
	Code        string
	Phase       string
	PlayerCount int
	CreatedAt   int64
}

// Hub 管理所有游戏房间（对应 TS Registry）
type Hub struct {
	mu                sync.RWMutex
	rooms             map[string]*Room
	store             RoomRepository
	cache             CacheStore
	timeouts          config.TimeoutConfig
	logger            *slog.Logger
	maxWSConnections  int
	wsConnCount       int64
	maxPlayersPerRoom int
	broadcaster       Broadcaster
	instanceID        string
	subscriptions     map[string]func()
}

// generateRoomCodeFn generates room codes; tests may replace it to simulate conflicts.
var generateRoomCodeFn = func() string { return GenerateRoomCode(defaultSeedRNG) }

// SetGenerateRoomCodeHook overrides room code generation in tests and returns a restore func.
func SetGenerateRoomCodeHook(fn func() string) (restore func()) {
	prev := generateRoomCodeFn
	generateRoomCodeFn = fn
	return func() { generateRoomCodeFn = prev }
}

// NewHub 创建房间注册中心
func NewHub(pgStore RoomRepository, cacheStore CacheStore, timeouts config.TimeoutConfig, maxWSConnections, maxPlayersPerRoom int, broadcaster Broadcaster) *Hub {
	if maxWSConnections <= 0 {
		maxWSConnections = config.MaxWSConnections
	}
	if maxPlayersPerRoom <= 0 {
		maxPlayersPerRoom = config.MaxPlayersPerRoom
	}
	return &Hub{
		rooms:             make(map[string]*Room),
		store:             pgStore,
		cache:             cacheStore,
		timeouts:          timeouts,
		logger:            slog.Default().With("component", "hub"),
		maxWSConnections:  maxWSConnections,
		maxPlayersPerRoom: maxPlayersPerRoom,
		broadcaster:       broadcaster,
		instanceID:        defaultInstanceID(),
		subscriptions:     make(map[string]func()),
	}
}

// CreateRoom 创建新房间，返回房间码
func (h *Hub) CreateRoom(ctx context.Context) (string, error) {
	h.mu.Lock()

	var code string
	for i := 0; i < 10; i++ {
		code = generateRoomCodeFn()
		if _, exists := h.rooms[code]; !exists {
			break
		}
		code = ""
	}
	if code == "" {
		h.mu.Unlock()
		return "", ErrRoomCodeConflict
	}

	room := NewRoom(code, h, h.store, h.timeouts, h.maxPlayersPerRoom)
	h.rooms[code] = room
	metrics.ActiveRooms.Set(float64(len(h.rooms)))
	h.mu.Unlock()

	h.subscribeRoom(code)
	h.registerRoomInRedis(code)
	h.invalidateLobbyReadCaches(code)

	audit.Log(ctx, audit.AuditEntry{
		Action:   "room.create",
		ActorID:  "system",
		Resource: "room/" + code,
		After:    map[string]interface{}{"code": code, "max_players": h.maxPlayersPerRoom},
	})

	h.logger.Info("room created", "code", code)
	metrics.ActiveRooms.Set(float64(len(h.rooms)))
	if _, err := domain.NewRoomCode(code); err != nil {
		h.logger.Error("generated invalid room code", "code", code, "error", err)
	}
	return code, nil
}

// GetRoom 获取房间（如果内存中没有则从数据库加载）
func (h *Hub) GetRoom(code string) *Room {
	h.mu.RLock()
	room, ok := h.rooms[code]
	h.mu.RUnlock()
	if ok {
		return room
	}

	// 双重检查锁定：避免 TOCTOU 竞态
	h.mu.Lock()
	room, ok = h.rooms[code]
	if ok {
		h.mu.Unlock()
		return room
	}
	room = h.loadOrMaterializeRoomLocked(code)
	h.mu.Unlock()
	if room != nil {
		h.finalizeMaterializedRoom(code)
	}
	return room
}

// RemoveRoom 移除房间
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
		h.unsubscribeRoomLocked(code)
		h.logger.Info("room closed on shutdown", "code", code)
	}
	metrics.ActiveRooms.Set(0)
}

// CheckRoom 检查房间是否存在
func (h *Hub) CheckRoom(code string) (*RoomInfo, error) {
	room := h.GetRoom(code)
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
	defer h.mu.RUnlock()
	total := 0
	for _, room := range h.rooms {
		room.mu.RLock()
		total += len(room.state.Players)
		room.mu.RUnlock()
	}
	return total
}

// PhaseCounts returns the number of rooms in each game phase.
func (h *Hub) PhaseCounts() map[string]int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	counts := make(map[string]int)
	for _, room := range h.rooms {
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
