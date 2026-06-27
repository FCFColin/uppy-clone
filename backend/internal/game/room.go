package game

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// PlayerConn 表示一个玩家的 WebSocket 连接
type PlayerConn struct {
	PlayerID string
	Conn     *websocket.Conn
	Send     chan []byte
	// consecutiveDrops tracks consecutive message drops for slow client detection.
	// P4-5: 连续丢弃计数，达到阈值后告警/断开慢客户端。访问由 Room.mu 保护。
	consecutiveDrops int
}

// Room 表示一个游戏房间。
//
// P3-5.1: Room 是 Aggregate Root，PlayerState 是其内部实体。
// 外部代码必须通过 Room 方法（AddPlayer、RemovePlayer、UpdatePlayerState）
// 修改玩家。直接访问 room.state.Players 字段是不推荐的。
//
// P3-5.3 Room 不变量（invariants）：
//   - Player count <= maxPlayersPerRoom
//   - Phase 转换必须遵循：waiting → countdown → playing → ended → waiting
//   - 同一房间内所有玩家昵称必须唯一
//
// P3-6.2: 领域事件（PlayerJoined/PlayerLeft/GameEnded/PhaseChanged，见 domain/events.go）
// 应通过 Transactional Outbox（P1-10）发布。当前未实际接入事件发布逻辑，
// 未来重构时在 AddPlayer/RemovePlayer/EndGame/阶段转换处生成事件并写入 outbox_events 表。
type Room struct {
	mu             sync.RWMutex
	state          *domain.GameState
	usedNames      map[string]bool
	connections    map[string]*PlayerConn // playerID → connection
	hub            *Hub
	store          RoomRepository
	timeouts       config.TimeoutConfig
	tickCancel     context.CancelFunc
	countdownStart int64
	logger         *slog.Logger
	maxPlayers     int // 每房间最大玩家数

	// players is a reusable slice for buildSnapshot to avoid allocating a new
	// slice on every snapshot (15 Hz per room). Access is guarded by mu.
	players []protocol.PlayerState

	// endGameAlarm 用于 ended 阶段的定时重启
	endGameTimer *time.Timer

	// wg tracks tick goroutines so Close() can wait for them to exit
	// before persisting state (P2-24: graceful shutdown).
	wg sync.WaitGroup

	// broadcaster 用于跨实例广播。nil 表示单实例模式（仅本地投递）。
	broadcaster Broadcaster
	// instanceID 标识当前实例，发布消息时写入 ExcludeInstance 防止 Pub/Sub 回环。
	instanceID string
}

// NewRoom 创建新房间
func NewRoom(code string, hub *Hub, repo RoomRepository, timeouts config.TimeoutConfig, maxPlayers int) *Room {
	if maxPlayers <= 0 {
		maxPlayers = config.MaxPlayersPerRoom
	}
	r := &Room{
		state:       NewGameState(code),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
		hub:         hub,
		store:       repo,
		timeouts:    timeouts,
		logger:      slog.Default().With("lobby", code),
		maxPlayers:  maxPlayers,
		instanceID:  defaultInstanceID(),
	}
	if hub != nil {
		r.broadcaster = hub.broadcaster
		r.instanceID = hub.instanceID
	}
	return r
}

// GetConnection returns the PlayerConn for a given playerID, or nil if not found.
func (r *Room) GetConnection(playerID string) *PlayerConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[playerID]
}

// Code returns the lobby code for this room.
func (r *Room) Code() string {
	return r.state.LobbyCode
}

// Close 清理房间，确保 tick goroutine 退出并持久化状态。
// 企业为何需要：优雅关闭时必须等待异步 tick goroutine 退出，避免写入已关闭的 channel
// 或持久化不完整状态。saveState 确保崩溃/关闭时房间状态可恢复。
func (r *Room) Close() {
	r.mu.Lock()
	r.stopTick()
	r.mu.Unlock()

	r.wg.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.endGameTimer != nil {
		r.endGameTimer.Stop()
	}
	for _, pc := range r.connections {
		_ = pc.Conn.Close()
		close(pc.Send)
	}
	r.connections = make(map[string]*PlayerConn)

	r.saveState()
}

// ErrRoomFull 房间玩家已满
var ErrRoomFull = &roomFullError{}

type roomFullError struct{}

func (e *roomFullError) Error() string { return "room is full" }
