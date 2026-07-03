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
	// pendingDisconnect marks a slow client for removal after outbound delivery.
	pendingDisconnect bool
}

// SnapshotTargets implements ConnectionSource.
func (r *Room) SnapshotTargets(excludePlayerID string) []connTarget {
	targets := make([]connTarget, 0, len(r.connections))
	for pid, pc := range r.connections {
		if pid == excludePlayerID || pc == nil || pc.Send == nil {
			continue
		}
		pcCopy := pc
		targets = append(targets, connTarget{
			playerID:          pid,
			send:              pcCopy.Send,
			consecutiveDrops:  &pcCopy.consecutiveDrops,
			pendingDisconnect: &pcCopy.pendingDisconnect,
			connClose: func() {
				if pcCopy.Conn != nil {
					_ = pcCopy.Conn.Close()
				}
			},
		})
	}
	return targets
}

// RemovePendingDisconnects implements ConnectionSource.
func (r *Room) RemovePendingDisconnects() {
	for pid, pc := range r.connections {
		if pc != nil && pc.pendingDisconnect {
			if player, ok := r.state.Players[pid]; ok {
				player.MarkDisconnected(time.Now().UnixMilli())
			}
			delete(r.connections, pid)
			pc.pendingDisconnect = false
		}
	}
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
	maxPlayers int // 每房间最大玩家数

	lobbyCode string // 房间码，不可变，在 NewRoom 中设置

	// players is a reusable slice for buildSnapshot to avoid allocating a new
	// slice on every snapshot (15 Hz per room). Access is guarded by mu.
	players []protocol.PlayerState

	// endGameAlarm 用于 ended 阶段的定时重启
	endGameAlarmVersion int64
	endGameTimer        *time.Timer

	// startDelayTimer 给玩家短暂时间看到欢迎信息后再开始倒计时
	startDelayTimer *time.Timer

	// startDelay 是开始游戏前的延迟，默认 1.5 秒，测试中可覆盖
	startDelay time.Duration

	// wg tracks tick goroutines so Close() can wait for them to exit
	// before persisting state (P2-24: graceful shutdown).
	wg sync.WaitGroup

	// asyncWg tracks outbound/persist worker goroutines.
	asyncWg sync.WaitGroup

	// outbound handles WebSocket message broadcasting (decoupled via interface).
	outbound *OutboundManager

	// syncOutbound delivers immediately (unit tests). Forwarded to OutboundManager.
	syncOutbound bool

	broadcaster Broadcaster
	instanceID  string

	persist        *PersistManager
	persistMu      sync.RWMutex
	lastPersistAt  time.Time
	persistCh      chan persistJob
}

// NewRoom 创建新房间
func NewRoom(code string, hub *Hub, repo RoomRepository, timeouts config.TimeoutConfig, maxPlayers int) *Room {
	if maxPlayers <= 0 {
		maxPlayers = config.MaxPlayersPerRoom
	}
	var broadcaster Broadcaster
	instanceID := defaultInstanceID()
	if hub != nil {
		broadcaster = hub.broadcaster
		instanceID = hub.instanceID
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
		instanceID:  instanceID,
		broadcaster: broadcaster,
		startDelay:  2000 * time.Millisecond,
		lobbyCode:   code,
	}
	r.outbound = NewOutboundManager(code, instanceID, &r.syncOutbound, broadcaster, r, r.logger, &r.asyncWg)
	return r
}

// GetConnection returns the PlayerConn for a given playerID, or nil if not found.
func (r *Room) GetConnection(playerID string) *PlayerConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[playerID]
}

// removeConnectionLocked 移除玩家连接（调用者须持有 r.mu）。
func (r *Room) removeConnectionLocked(playerID string) {
	if pc, ok := r.connections[playerID]; ok {
		if pc.Conn != nil {
			_ = pc.Conn.Close()
		}
		delete(r.connections, playerID)
	}
}

// removeConnection 线程安全移除玩家连接。
func (r *Room) removeConnection(playerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeConnectionLocked(playerID)
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

	r.stopOutbound()

	r.mu.Lock()
	if r.endGameTimer != nil {
		r.endGameTimer.Stop()
	}
	if r.startDelayTimer != nil {
		r.startDelayTimer.Stop()
	}
	for pid, pc := range r.connections {
		r.removeConnectionLocked(pid)
		close(pc.Send)
	}
	r.connections = make(map[string]*PlayerConn)
	r.mu.Unlock()

	r.flushPersistSync()
	r.stopPersist()
	r.asyncWg.Wait()
}

// ErrRoomFull 房间玩家已满
var ErrRoomFull = &roomFullError{}

type roomFullError struct{}

func (e *roomFullError) Error() string { return "room is full" }

// SerializeStateJSON implements PersistSource.
func (r *Room) SerializeStateJSON() ([]byte, string, error) {
	if r.state == nil {
		return nil, "", nil
	}
	data, err := serializeStateFn(r.state)
	if err != nil {
		return nil, "", err
	}
	return data, r.state.LobbyCode, nil
}

// Store implements PersistSource.
func (r *Room) Store() RoomRepository { return r.store }

// LobbyCode implements PersistSource.
func (r *Room) LobbyCode() string { return r.lobbyCode }

// Timeouts implements PersistSource.
func (r *Room) Timeouts() config.TimeoutConfig { return r.timeouts }
