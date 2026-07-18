package game

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("github.com/uppy-clone/backend/internal/game")

// PlayerConn 表示一个玩家的 WebSocket 连接
type PlayerConn struct {
	PlayerID string
	Conn     *websocket.Conn
	Send     chan []byte
	// consecutiveDrops tracks consecutive message drops for slow client detection.
	// P4-5: 连续丢弃计数，达到阈值后告警/断开慢客户端。
	// Protected by atomic operations (accessed concurrently by outbound goroutine
	// without holding connMu).
	consecutiveDrops atomic.Int64
	// pendingDisconnect marks a slow client for removal after outbound delivery.
	pendingDisconnect atomic.Bool
}

// SendChannel returns the outbound message channel for interface access.
func (p *PlayerConn) SendChannel() chan []byte { return p.Send }

// RoomConnections manages the per-room WebSocket connection registry.
// Embedded in Room; has its own mutex (connMu) to avoid deadlock with the
// outbound goroutine, which must not acquire Room.mu.
type RoomConnections struct {
	connMu      sync.RWMutex
	connections map[string]*PlayerConn
}

// GetConnection returns the PlayerConn associated with the given player ID, or nil if not found.
func (rc *RoomConnections) GetConnection(playerID string) *PlayerConn {
	rc.connMu.RLock()
	defer rc.connMu.RUnlock()
	return rc.connections[playerID]
}

// removeConnectionLocked closes and removes a player connection.
// Caller must NOT hold connMu (acquires it internally).
func (rc *RoomConnections) removeConnectionLocked(playerID string) {
	rc.connMu.Lock()
	defer rc.connMu.Unlock()
	if pc, ok := rc.connections[playerID]; ok {
		if pc.Conn != nil {
			_ = pc.Conn.Close()
		}
		delete(rc.connections, playerID)
	}
}

func (rc *RoomConnections) sendToPlayer(playerID string, data []byte) {
	rc.connMu.RLock()
	pc, ok := rc.connections[playerID]
	rc.connMu.RUnlock()
	if ok {
		select {
		case pc.Send <- data:
		default:
		}
	}
}

// SnapshotTargets returns a snapshot of all connection targets except the excluded player.
func (rc *RoomConnections) SnapshotTargets(excludePlayerID string) []connTarget {
	rc.connMu.RLock()
	defer rc.connMu.RUnlock()
	targets := make([]connTarget, 0, len(rc.connections))
	for pid, pc := range rc.connections {
		if pid == excludePlayerID || pc == nil || pc.Send == nil {
			continue
		}
		targets = append(targets, connTarget{
			playerID:          pid,
			send:              pc.Send,
			consecutiveDrops:  &pc.consecutiveDrops,
			pendingDisconnect: &pc.pendingDisconnect,
			connClose: func() {
				if pc.Conn != nil {
					_ = pc.Conn.Close()
				}
			},
		})
	}
	return targets
}

func (rc *RoomConnections) closeAllConnections() {
	rc.connMu.Lock()
	for pid, pc := range rc.connections {
		if pc.Conn != nil {
			_ = pc.Conn.Close()
		}
		delete(rc.connections, pid)
		close(pc.Send)
	}
	rc.connections = make(map[string]*PlayerConn)
	rc.connMu.Unlock()
}

// RemovePendingDisconnects removes slow clients flagged for disconnection.
// In async (production) mode, acquires r.mu to protect r.state.Players access.
// In sync (test) mode where r.mu may already be held, TryLock gracefully skips
// player state marking since there's no concurrent access in the same goroutine.
//
// This method stays on Room (not RoomConnections) because it bridges the
// connection registry and game state: it marks disconnected players in
// r.state.Players while also deleting from the connection map.
func (r *Room) RemovePendingDisconnects() {
	hasMu := r.mu.TryLock()
	if hasMu {
		defer r.mu.Unlock()
	}
	r.connMu.Lock()
	defer r.connMu.Unlock()
	for pid, pc := range r.connections {
		if pc != nil && pc.pendingDisconnect.Load() {
			if hasMu {
				if player, ok := r.state.Players[pid]; ok {
					player.MarkDisconnected(time.Now().UnixMilli())
				}
			}
			delete(r.connections, pid)
		}
	}
}

// ─── Broadcast ───────────────────────────────────────────────────────

// broadcast sends data to all connections (optionally excluding one player).
// Caller must hold r.mu. Actual delivery happens in the outbound goroutine (lock-free).
func (r *Room) broadcast(data []byte, excludePlayerID string) {
	r.enqueueOutbound(data, broadcastOpts{excludePlayerID: excludePlayerID})
}

// broadcastCritical sends a critical phase message with blocking delivery per client.
// 调用方必须持有 r.mu 锁。
func (r *Room) broadcastCritical(message []byte) {
	r.enqueueOutbound(message, broadcastOpts{critical: true})
}

// broadcastOpts controls outbound delivery behavior.
type broadcastOpts struct {
	excludePlayerID string
	critical        bool
}

// enqueueOutbound queues a broadcast for async delivery. Caller must hold r.mu.
func (r *Room) enqueueOutbound(payload []byte, opts broadcastOpts) {
	r.initOutboundManager()
	r.outbound.Enqueue(payload, opts.excludePlayerID, opts.critical)
}

func (r *Room) initOutboundManager() {
	if r.outbound == nil {
		r.outbound = NewOutboundManager(r.lobbyCode, &r.syncOutbound, r, r.logger, &r.asyncWg)
	}
}

// stopOutbound stops the outbound delivery loop.
func (r *Room) stopOutbound() {
	if r.outbound == nil {
		return
	}
	r.outbound.Stop()
}

// snapshotData holds a copy of game state for lock-free snapshot encoding.
type snapshotData struct {
	phase     protocol.GamePhase
	tickCount uint32
	score     uint32
	balloon   protocol.BalloonState
	bird      protocol.BirdState
	ghost     protocol.GhostState
	players   []protocol.PlayerState
	wind      float64
}

// extractSnapshotDataLocked copies the state needed for a snapshot.
// Caller must hold r.mu.
func (r *Room) extractSnapshotDataLocked() snapshotData {
	now := time.Now().UnixMilli()
	players := make([]protocol.PlayerState, 0, len(r.state.Players))
	for _, p := range r.state.Players {
		if p.Disconnected {
			continue
		}
		cooldownRemaining := int64(0)
		if p.CooldownEndTime > now {
			cooldownRemaining = p.CooldownEndTime - now
		}
		players = append(players, protocol.PlayerState{
			PlayerIndex:       uint16(p.PlayerIndex),       //nolint:gosec // G115: PlayerIndex < MaxPlayersPerRoom(50)
			CooldownMs:        uint32(cooldownRemaining),   //nolint:gosec // G115: bounded by cooldown duration
			Palette:           uint32(p.Palette),           //nolint:gosec // G115: Palette < 10
			ScoreContribution: uint32(p.ScoreContribution), //nolint:gosec // G115: score bounded by game logic
			Nickname:          p.Nickname,
		})
	}
	return snapshotData{
		phase:     protocol.GamePhase(r.state.Phase),
		tickCount: uint32(r.state.TickCount),     //nolint:gosec // G115: tick count bounded by game session
		score:     uint32(r.state.Balloon.Score), //nolint:gosec // G115: score incremented one at a time, bounded by game session
		balloon: protocol.BalloonState{
			X:  float32(r.state.Balloon.X),
			Y:  float32(r.state.Balloon.Y),
			Vy: float32(r.state.Balloon.VY),
			Vx: float32(r.state.Balloon.VX),
		},
		bird: protocol.BirdState{
			X:      float32(r.state.Bird.X),
			Y:      float32(r.state.Bird.Y),
			Active: r.state.Bird.Active,
		},
		ghost: protocol.GhostState{
			X:          float32(r.state.Ghost.X),
			Y:          float32(r.state.Ghost.Y),
			Active:     r.state.Ghost.Active,
			RepelTimer: uint16(r.state.Ghost.RepelTimer), //nolint:gosec // G115: repel timer bounded by game logic
		},
		players: players,
		wind:    r.state.Wind,
	}
}

// encodeSnapshot encodes a snapshot from pre-captured data.
// Safe to call without holding r.mu.
func encodeSnapshot(sd snapshotData) []byte {
	return protocol.EncodeSnapshot(sd.phase, sd.tickCount, sd.score, sd.balloon, sd.bird, sd.ghost, sd.players, nil, sd.wind)
}

// buildSnapshot encodes the current state as a snapshot.
// Caller must hold r.mu.
func (r *Room) buildSnapshot() []byte {
	sd := r.extractSnapshotDataLocked()
	return encodeSnapshot(sd)
}

// ─── Room Aggregate Root ─────────────────────────────────────────────

// Room represents a game room and is the aggregate root.
//
// Invariants:
//   - Player count <= maxPlayersPerRoom
//   - Phase transitions follow: waiting → countdown → playing → ended → waiting
//   - All player nicknames in a room must be unique
//
// Domain events (PlayerJoined/PlayerLeft/GameEnded/PhaseChanged) should eventually
// be published through the Transactional Outbox pattern.
type Room struct {
	mu              sync.RWMutex
	RoomConnections // embedded: connMu + connections map + connection methods (promoted)

	endGameAlarmVersion int64
	endGameTimer        *time.Timer
	startDelayTimer     *time.Timer
	startDelay          time.Duration // 开始游戏前的延迟，默认 1.5 秒，测试中可覆盖
	countdownStart      int64         // countdown phase 开始时间 (unix milli)
	tickCancel          context.CancelFunc

	outbound     *OutboundManager
	syncOutbound bool // true = immediate delivery (unit tests)

	state      *domain.GameState
	usedNames  map[string]bool
	hub        *Hub
	store      RoomRepository
	timeouts   config.TimeoutConfig
	logger     *slog.Logger
	maxPlayers int // 每房间最大玩家数

	lobbyCode string // 房间码，不可变，在 NewRoom 中设置

	// closed marks the room as shutting down; checked by timer callbacks to prevent ghost restarts.
	closed atomic.Bool

	// wg tracks tick goroutines so Close() can wait for them to exit
	// before persisting state (P2-24: graceful shutdown).
	wg sync.WaitGroup

	// asyncWg tracks outbound/persist worker goroutines.
	asyncWg sync.WaitGroup

	persist *PersistManager

	// rng is the per-room deterministic RNG for game ticks.
	// Seed is stored in GameState for replayability.
	rng RNGSource
}

// NewRoom 创建新房间
func NewRoom(code string, hub *Hub, repo RoomRepository, timeouts config.TimeoutConfig, maxPlayers int) *Room {
	_, span := tracer.Start(context.Background(), "game.new_room")
	defer span.End()
	span.SetAttributes(attribute.String("lobby.code", code))
	if maxPlayers <= 0 {
		maxPlayers = config.MaxPlayersPerRoom
	}
	seed := time.Now().UnixNano()
	roomRNG := newSeededRNG(seed)
	r := &Room{
		RoomConnections: RoomConnections{
			connections: make(map[string]*PlayerConn),
		},
		startDelay: 2000 * time.Millisecond,
		state:      NewGameState(code, seed, roomRNG),
		usedNames:  make(map[string]bool),
		hub:        hub,
		store:      repo,
		timeouts:   timeouts,
		logger:     slog.Default().With("lobby", code),
		maxPlayers: maxPlayers,
		lobbyCode:  code,
		rng:        roomRNG,
	}
	r.outbound = NewOutboundManager(code, &r.syncOutbound, r, r.logger, &r.asyncWg)
	r.persist = newPersistManager(r, r.logger, &r.asyncWg)
	return r
}

// Code returns the lobby code for this room.
func (r *Room) Code() string {
	return string(r.state.LobbyCode)
}

// RunSession delegates to WSSession to keep Room focused on game state.
func (r *Room) RunSession(reqCtx context.Context, playerID string, conn *websocket.Conn) error {
	return (&WSSession{room: r}).RunSession(reqCtx, playerID, conn)
}

// Close cleans up the room, ensuring the tick goroutine exits and state is persisted.
func (r *Room) Close() {
	_, span := tracer.Start(context.Background(), "game.room_close")
	defer span.End()
	span.SetAttributes(attribute.String("lobby.code", r.lobbyCode))
	r.closed.Store(true)
	r.mu.Lock()
	r.stopTick()
	r.mu.Unlock()

	// Wait for tick goroutine to exit, with a timeout to prevent hanging.
	// game-031: If the timeout fires, the goroutine running r.wg.Wait() will leak.
	// This is an acceptable trade-off: the subsequent cleanup (closing connections,
	// stopping tick) will cause the tick goroutine to exit, which decrements r.wg,
	// allowing the leaked goroutine to also exit. A context-cancellable WaitGroup
	// would add complexity for minimal benefit.
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		r.logger.Warn("tick goroutine did not exit within timeout", "lobby", r.lobbyCode)
	}

	r.stopOutbound()

	r.mu.Lock()
	if r.endGameTimer != nil {
		r.endGameTimer.Stop()
	}
	if r.startDelayTimer != nil {
		r.startDelayTimer.Stop()
	}
	r.closeAllConnections()
	r.mu.Unlock()

	r.flushPersistSync()
	r.stopPersist()
	// Wait for async workers with a timeout to prevent hanging on shutdown.
	asyncDone := make(chan struct{})
	go func() {
		r.asyncWg.Wait()
		close(asyncDone)
	}()
	select {
	case <-asyncDone:
	case <-time.After(10 * time.Second):
		r.logger.Warn("async workers did not exit within timeout", "lobby", r.lobbyCode)
	}
}

// ErrRoomFull 房间玩家已满
var ErrRoomFull = &roomFullError{}

type roomFullError struct{}

func (e *roomFullError) Error() string { return "room is full" }

// SerializeStateJSON serializes the room state for persistence.
func (r *Room) SerializeStateJSON() ([]byte, string, error) {
	if r.state == nil {
		return nil, "", nil
	}
	data, err := SerializeState(r.state)
	if err != nil {
		return nil, "", err
	}
	return data, string(r.state.LobbyCode), nil
}

// Store returns the room's repository.
func (r *Room) Store() RoomRepository { return r.store }

// LobbyCode returns the room's lobby code.
func (r *Room) LobbyCode() string { return r.lobbyCode }

// Timeouts returns the room's timeout configuration.
func (r *Room) Timeouts() config.TimeoutConfig { return r.timeouts }

// Observer returns the GameObserver from the owning Hub.
// Satisfies the OutboundSource interface.
func (r *Room) Observer() GameObserver {
	if r.hub != nil {
		return r.hub.Observer()
	}
	return NoopGameObserver{}
}
