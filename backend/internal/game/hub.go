// Package game implements the multiplayer game engine.
package game

import (
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
	"github.com/uppy-clone/backend/internal/store"
)

// RoomInfo 房间摘要信息
type RoomInfo struct {
	Code        string
	Phase       string
	PlayerCount int
	CreatedAt   int64
}

// Hub 管理所有游戏房间（对应 TS Registry）
//
// 企业为何需要：舱壁隔离（Bulkhead）防止单类资源耗尽拖垮整体。WebSocket 连接洪水可耗尽文件描述符和内存，
// 导致 REST API 也无法响应。连接上限是 DoS 防御的基本措施。
//
// P3-3.5: 未来重构应通过依赖注入传入 RoomRepository 与 SnapshotEncoder 接口，
// 移除对 store/protocol 包的直接依赖（见 repository.go）。
type Hub struct {
	mu                sync.RWMutex
	rooms             map[string]*Room // lobbyCode → Room
	store             *store.PostgresStore
	redis             *store.RedisStore
	timeouts          config.TimeoutConfig
	logger            *slog.Logger
	maxWSConnections  int   // 全局 WebSocket 连接上限
	wsConnCount       int64 // 当前 WebSocket 连接数（atomic 操作）
	maxPlayersPerRoom int   // 每房间最大玩家数

	// broadcaster 用于跨实例广播（ADR-005）。nil 表示单实例模式。
	broadcaster Broadcaster
	// instanceID 标识当前实例，用于过滤 Pub/Sub 回环消息。
	instanceID string
	// subscriptions 存储每个房间的取消订阅函数（roomCode → unsubscribe）。
	subscriptions map[string]func()
}

// NewHub 创建房间注册中心
// broadcaster 可为 nil（单实例模式/测试场景）。
func NewHub(pgStore *store.PostgresStore, redisStore *store.RedisStore, timeouts config.TimeoutConfig, maxWSConnections, maxPlayersPerRoom int, broadcaster Broadcaster) *Hub {
	if maxWSConnections <= 0 {
		maxWSConnections = config.MaxWSConnections
	}
	if maxPlayersPerRoom <= 0 {
		maxPlayersPerRoom = config.MaxPlayersPerRoom
	}
	return &Hub{
		rooms:             make(map[string]*Room),
		store:             pgStore,
		redis:             redisStore,
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
// 企业为何需要：传入请求 context 以便审计日志关联 trace_id/request_id，
// 满足 SOC2 审计追溯要求。原 context.Background() 丢失了请求链路信息。
func (h *Hub) CreateRoom(ctx context.Context) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 生成唯一房间码（最多重试 10 次）
	var code string
	for i := 0; i < 10; i++ {
		code = GenerateRoomCode()
		if _, exists := h.rooms[code]; !exists {
			break
		}
		code = ""
	}
	if code == "" {
		return "", ErrRoomCodeConflict
	}

	room := NewRoom(code, h, h.store, h.timeouts, h.maxPlayersPerRoom)
	h.rooms[code] = room

	// 订阅跨实例广播频道（ADR-005）
	h.subscribeRoom(code)

	// Register room in Redis for multi-instance discovery (ADR-005)
	h.registerRoomInRedis(code)

	// Audit: room creation
	audit.Log(ctx, audit.AuditEntry{
		Action:   "room.create",
		ActorID:  "system",
		Resource: "room/" + code,
		After:    map[string]interface{}{"code": code, "max_players": h.maxPlayersPerRoom},
	})

	h.logger.Info("room created", "code", code)
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

	// 尝试从数据库加载
	if h.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGQueryTimeout)
		defer cancel()

		ls, err := h.store.LoadLobbyState(ctx, code)
		if err != nil || ls == nil {
			return nil
		}

		state, err := DeserializeState([]byte(ls.State))
		if err != nil {
			h.logger.Error("deserialize lobby state", "code", code, "error", err)
			return nil
		}

		room := NewRoom(code, h, h.store, h.timeouts, h.maxPlayersPerRoom)
		room.state = state
		// 重建 usedNames
		for _, p := range state.Players {
			room.usedNames[p.Nickname] = true
		}

		h.mu.Lock()
		// 双重检查
		if existing, ok := h.rooms[code]; ok {
			h.mu.Unlock()
			return existing
		}
		h.rooms[code] = room
		h.subscribeRoom(code)
		h.mu.Unlock()

		return room
	}

	return nil
}

// RemoveRoom 移除房间
// 企业为何需要：传入请求 context 以便审计日志关联 trace_id/request_id，
// 满足 SOC2 审计追溯要求。原 context.Background() 丢失了请求链路信息。
func (h *Hub) RemoveRoom(ctx context.Context, code string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if room, ok := h.rooms[code]; ok {
		room.Close()
		delete(h.rooms, code)
		h.unsubscribeRoom(code)

		// Audit: room deletion
		audit.Log(ctx, audit.AuditEntry{
			Action:   "room.delete",
			ActorID:  "system",
			Resource: "room/" + code,
			Before:   map[string]interface{}{"code": code},
		})

		h.logger.Info("room removed", "code", code)
	}

	// 从数据库也删除
	if h.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGQueryTimeout)
		defer cancel()
		_ = h.store.DeleteLobbyState(ctx, code)
	}

	// 从 Redis 注销房间（多实例发现 + 缓存）
	h.unregisterRoomFromRedis(code)
}

// CloseAllRooms closes all active rooms, ensuring state is persisted.
// 企业为何需要：优雅关闭时必须持久化所有房间状态，避免数据丢失。
// 在服务器收到 SIGTERM 时调用，确保 tick goroutine 退出且状态写入 DB。
func (h *Hub) CloseAllRooms() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for code, room := range h.rooms {
		room.Close()
		delete(h.rooms, code)
		h.unsubscribeRoom(code)
		h.logger.Info("room closed on shutdown", "code", code)
	}
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
		Code:        room.state.LobbyCode,
		Phase:       string(room.state.Phase),
		PlayerCount: len(room.state.Players),
		CreatedAt:   room.state.StartedAt,
	}, nil
}

// RestoreRooms 从 PostgreSQL 加载所有活跃房间（启动时调用）
func (h *Hub) RestoreRooms() error {
	if h.store == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGRequestTimeout)
	defer cancel()

	// Load all lobbies using cursor-based pagination (no cursor = first page)
	result, err := h.store.LoadAllActiveLobbies(ctx, 100, "")
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, ls := range result.Lobbies {
		if _, ok := h.rooms[ls.Code]; ok {
			continue
		}

		state, err := DeserializeState([]byte(ls.State))
		if err != nil {
			h.logger.Error("deserialize lobby state on restore", "code", ls.Code, "error", err)
			continue
		}

		room := NewRoom(ls.Code, h, h.store, h.timeouts, h.maxPlayersPerRoom)
		room.state = state
		for _, p := range state.Players {
			room.usedNames[p.Nickname] = true
		}

		h.rooms[ls.Code] = room
		h.subscribeRoom(ls.Code)
		h.logger.Info("restored room", "code", ls.Code, "phase", state.Phase)
	}

	h.logger.Info("rooms restored", "count", len(h.rooms))
	return nil
}

// CleanupLoop 定期清理空房间
func (h *Hub) CleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(config.CleanupInterval)
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

// cleanupOnce 执行一次空房间清理
//
// P4-3: 采用"快照 + 锁外处理"模式，减少 hub 写锁持有时间。
// 原实现全程持有 h.mu.Lock()，阻塞 CreateRoom/GetRoom。
// 新实现分三步：(1) RLock 快照房间码；(2) 锁外逐个检查房间状态；
// (3) 短暂 Lock 删除空房间。检查阶段不阻塞读写操作。
func (h *Hub) cleanupOnce() {
	// Step 1: Snapshot room codes with read lock
	codes := h.snapshotRoomCodes()

	now := time.Now().UnixMilli()

	// Step 2: Check each room without holding hub lock
	var toCleanup []string
	for _, code := range codes {
		h.mu.RLock()
		room, ok := h.rooms[code]
		h.mu.RUnlock()
		if !ok {
			continue
		}

		if shouldCleanupRoom(room, now) {
			toCleanup = append(toCleanup, code)
		}
	}

	// Step 3: Delete empty rooms with brief write lock
	h.deleteRooms(toCleanup)
}

// snapshotRoomCodes returns a snapshot of all room codes under a read lock.
func (h *Hub) snapshotRoomCodes() []string {
	h.mu.RLock()
	codes := make([]string, 0, len(h.rooms))
	for code := range h.rooms {
		codes = append(codes, code)
	}
	h.mu.RUnlock()
	return codes
}

// shouldCleanupRoom determines whether a room should be cleaned up based on its
// phase, player count, and connection/disconnection state.
func shouldCleanupRoom(room *Room, now int64) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()

	phase := room.state.Phase
	playerCount := len(room.state.Players)
	hasConnections := len(room.connections) > 0

	// 清理条件：waiting 阶段且无连接
	if phase == domain.PhaseWaiting && !hasConnections {
		return true
	}

	// 无玩家且无连接
	if playerCount == 0 && !hasConnections {
		return true
	}

	// 检查是否所有玩家都已断连超过优雅期
	if !hasConnections && playerCount > 0 {
		return allPlayersDisconnectedExpired(room.state.Players, now)
	}
	return false
}

// allPlayersDisconnectedExpired returns true if every player is disconnected
// and has been so for longer than the 60-second grace period.
func allPlayersDisconnectedExpired(players map[string]*domain.PlayerState, now int64) bool {
	for _, p := range players {
		if p.Disconnected && p.DisconnectedAt != nil {
			if now-*p.DisconnectedAt <= 60_000 { // 60 秒额外宽限
				return false
			}
		} else {
			return false
		}
	}
	return true
}

// deleteRooms removes the given room codes under a brief write lock.
func (h *Hub) deleteRooms(codes []string) {
	if len(codes) == 0 {
		return
	}
	h.mu.Lock()
	for _, code := range codes {
		if room, ok := h.rooms[code]; ok {
			room.Close()
			delete(h.rooms, code)
			h.unsubscribeRoom(code)
			h.unregisterRoomFromRedis(code)
			h.logger.Info("cleaned up empty room", "code", code)
		}
	}
	h.mu.Unlock()
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

// DB returns the underlying PostgreSQL store.
//
// Deprecated: Use specific Hub methods instead of accessing the store directly.
// This will be removed once all handlers are migrated to service-layer methods.
func (h *Hub) DB() *store.PostgresStore {
	return h.store
}

// ListLobbies returns active lobbies with cursor-based pagination.
// 企业为何需要（T24）：封装 DB 访问，handler 不再直接依赖 store 层。
func (h *Hub) ListLobbies(ctx context.Context, limit int, cursor string) (*store.LobbyListResult, error) {
	if h.store == nil {
		return nil, fmt.Errorf("store not available")
	}
	return h.store.LoadAllActiveLobbies(ctx, limit, cursor)
}

// Timeouts returns the timeout configuration.
func (h *Hub) Timeouts() config.TimeoutConfig {
	return h.timeouts
}

// registerRoomInRedis stores room metadata in Redis for multi-instance discovery.
// Enterprise rationale: Room state in Redis enables multi-instance deployment.
// When Hub instance A creates a room, instance B can discover it via Redis.
// This is the first step toward horizontal scaling per ADR-005.
// Trade-off: Extra Redis round-trip per room create/destroy, but enables
// future multi-instance deployment.
func (h *Hub) registerRoomInRedis(code string) {
	if h.redis == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.RedisConnectTimeout)
	defer cancel()

	data, _ := json.Marshal(map[string]interface{}{
		"code":       code,
		"created_at": time.Now().UnixMilli(),
		"instance":   os.Getenv("INSTANCE_ID"),
	})
	_ = h.redis.RegisterRoom(ctx, code, data, 24*time.Hour)
}

// unregisterRoomFromRedis removes room metadata from Redis.
func (h *Hub) unregisterRoomFromRedis(code string) {
	if h.redis == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.RedisConnectTimeout)
	defer cancel()
	_ = h.redis.UnregisterRoom(ctx, code)
}

// subscribeRoom 订阅房间的跨实例广播频道。
// 调用方必须持有 h.mu 锁。broadcaster 为 nil 时为空操作。
func (h *Hub) subscribeRoom(code string) {
	if h.broadcaster == nil {
		return
	}
	if _, exists := h.subscriptions[code]; exists {
		return
	}
	unsub, err := h.broadcaster.Subscribe(code, func(msg BroadcastMessage) {
		h.handleRemoteBroadcast(code, msg)
	})
	if err != nil {
		h.logger.Warn("subscribe room broadcast failed", "code", code, "error", err)
		return
	}
	h.subscriptions[code] = unsub
}

// unsubscribeRoom 取消房间的跨实例广播订阅。
// 调用方必须持有 h.mu 锁。
func (h *Hub) unsubscribeRoom(code string) {
	if unsub, ok := h.subscriptions[code]; ok {
		unsub()
		delete(h.subscriptions, code)
	}
}

// handleRemoteBroadcast 处理来自其他实例的 Redis Pub/Sub 广播消息。
// 跳过由本实例发出的消息（ExcludeInstance 匹配），避免回环。
func (h *Hub) handleRemoteBroadcast(roomCode string, msg BroadcastMessage) {
	if msg.ExcludeInstance == h.instanceID {
		return
	}
	h.mu.RLock()
	room, ok := h.rooms[roomCode]
	h.mu.RUnlock()
	if !ok {
		return
	}
	room.mu.Lock()
	room.broadcastLocal(msg.Payload, msg.ExcludePlayer)
	room.mu.Unlock()
}

// 错误定义
// ErrRoomCodeConflict is returned when a room code collision occurs.
var ErrRoomCodeConflict = &roomCodeConflictError{}

type roomCodeConflictError struct{}

func (e *roomCodeConflictError) Error() string { return "room code conflict after 10 retries" }

// ErrWSConnectionLimit 全局 WebSocket 连接数已达上限
var ErrWSConnectionLimit = &wsConnectionLimitError{}

type wsConnectionLimitError struct{}

func (e *wsConnectionLimitError) Error() string { return "websocket connection limit reached" }

// CanAcceptWSConnection 检查是否可以接受新的 WebSocket 连接
// 企业为何需要：舱壁隔离（Bulkhead）防止单类资源耗尽拖垮整体。WebSocket 连接洪水可耗尽文件描述符和内存，
// 导致 REST API 也无法响应。连接上限是 DoS 防御的基本措施。
func (h *Hub) CanAcceptWSConnection() bool {
	return atomic.LoadInt64(&h.wsConnCount) < int64(h.maxWSConnections)
}

// IncrementWSConnection 原子递增 WebSocket 连接计数
func (h *Hub) IncrementWSConnection() {
	count := atomic.AddInt64(&h.wsConnCount, 1)
	metrics.WSConnections.Set(float64(count))
}

// DecrementWSConnection 原子递减 WebSocket 连接计数
func (h *Hub) DecrementWSConnection() {
	count := atomic.AddInt64(&h.wsConnCount, -1)
	metrics.WSConnections.Set(float64(count))
}

// WSConnCount 返回当前 WebSocket 连接数
func (h *Hub) WSConnCount() int64 {
	return atomic.LoadInt64(&h.wsConnCount)
}

// MaxWSConnections 返回全局 WebSocket 连接上限
func (h *Hub) MaxWSConnections() int {
	return h.maxWSConnections
}

// MaxPlayersPerRoom 返回每房间最大玩家数
func (h *Hub) MaxPlayersPerRoom() int {
	return h.maxPlayersPerRoom
}
