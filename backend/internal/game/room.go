package game

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/idgen"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/store"
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
	store          *store.PostgresStore
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
func NewRoom(code string, hub *Hub, pgStore *store.PostgresStore, timeouts config.TimeoutConfig, maxPlayers int) *Room {
	if maxPlayers <= 0 {
		maxPlayers = config.MaxPlayersPerRoom
	}
	r := &Room{
		state:       NewGameState(code),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
		hub:         hub,
		store:       pgStore,
		timeouts:    timeouts,
		logger:      slog.Default().With("lobby", code),
		maxPlayers:  maxPlayers,
		instanceID:  defaultInstanceID(),
	}
	// 从 Hub 继承 broadcaster 与 instanceID，保证同一实例内一致
	if hub != nil {
		r.broadcaster = hub.broadcaster
		r.instanceID = hub.instanceID
	}
	return r
}

// HandleJoin 处理玩家加入/重连
// 企业为何需要：舱壁隔离（Bulkhead）防止单类资源耗尽拖垮整体。WebSocket 连接洪水可耗尽文件描述符和内存，
// 导致 REST API 也无法响应。连接上限是 DoS 防御的基本措施。
func (r *Room) HandleJoin(playerID string, conn *websocket.Conn) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.state.Players[playerID]
	isReconnect := player != nil

	r.closeExistingConnection(playerID, player)

	pc := &PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Send:     make(chan []byte, config.WSChannelBuffer),
	}
	r.connections[playerID] = pc

	if player != nil && player.Disconnected {
		r.reconnectPlayer(playerID, player)
		return nil
	}

	if player == nil {
		newPlayer, err := r.addNewPlayer(playerID, conn)
		if err != nil {
			return err
		}
		player = newPlayer
	}

	r.notifyJoin(playerID, player, isReconnect)
	r.transitionPhaseIfNeeded()

	return nil
}

// closeExistingConnection 关闭玩家已有的活跃连接（重连场景下替换旧连接）。
func (r *Room) closeExistingConnection(playerID string, player *domain.PlayerState) {
	if player == nil || player.Disconnected {
		return
	}
	if oldConn, ok := r.connections[playerID]; ok {
		r.logger.Info("closing old WebSocket for player", "playerID", playerID)
		oldConn.Conn.Close()
		delete(r.connections, playerID)
	}
}

// reconnectPlayer 处理断连优雅期内的重连：恢复状态、发送快照、重置定时器。
func (r *Room) reconnectPlayer(playerID string, player *domain.PlayerState) {
	player.Disconnected = false
	player.DisconnectedAt = nil
	r.logger.Info("player reconnected during grace period", "playerID", playerID)
	r.sendToPlayer(playerID, r.buildSnapshot())
	r.saveState()
	if r.state.Phase == domain.PhasePlaying && r.tickCancel == nil {
		r.startTick()
	}
	if r.state.Phase == domain.PhaseCountdown && r.tickCancel == nil {
		elapsed := time.Now().UnixMilli() - r.countdownStart
		countdownMs := int64(protocol.CountdownTicks) * 1000 / int64(protocol.TickRate)
		remaining := countdownMs - elapsed
		if remaining < 100 {
			remaining = 100
		}
		r.scheduleCountdownEnd(time.Now().Add(time.Duration(remaining) * time.Millisecond))
	}
}

// addNewPlayer 添加新玩家到房间，房间已满时返回 ErrRoomFull。
func (r *Room) addNewPlayer(playerID string, conn *websocket.Conn) (*domain.PlayerState, error) {
	if len(r.state.Players) >= r.maxPlayers {
		delete(r.connections, playerID)
		if conn != nil {
			conn.Close()
		}
		return nil, ErrRoomFull
	}

	palette := r.state.NextPlayerIndex % 10
	now := time.Now().UnixMilli()
	nickname := SanitizePlayerName(GenerateUniqueNickname("", r.usedNames))

	player := &domain.PlayerState{
		ID:                 playerID,
		PlayerIndex:        r.state.NextPlayerIndex,
		Nickname:           nickname,
		Palette:            palette,
		CooldownEndTime:    now,
		ScoreContribution:  0,
		TapsCount:          0,
		MessageCount:       0,
		MessageWindowStart: 0,
		LastNicknameChange: 0,
	}
	r.state.NextPlayerIndex++
	r.state.Players[playerID] = player
	r.usedNames[nickname] = true
	return player, nil
}

// notifyJoin 发送完整状态给玩家并广播 player_join 给其他玩家。
func (r *Room) notifyJoin(playerID string, player *domain.PlayerState, isReconnect bool) {
	if isReconnect {
		r.logger.Info("player reconnect", "playerID", playerID, "index", player.PlayerIndex)
	} else {
		r.logger.Info("player join", "playerID", playerID, "index", player.PlayerIndex)
	}

	r.sendToPlayer(playerID, r.buildSnapshot())

	joinMsg := protocol.EncodePlayerJoin(uint16(player.PlayerIndex), player.Nickname, uint32(player.Palette)) //nolint:gosec // PlayerIndex < MaxPlayersPerRoom, Palette < 8
	r.broadcast(joinMsg, playerID)

	r.saveState()
}

// transitionPhaseIfNeeded 检查并执行阶段转换（waiting→countdown 或恢复 tick）。
func (r *Room) transitionPhaseIfNeeded() {
	if r.state.Phase == domain.PhaseWaiting && len(r.state.Players) > 0 {
		r.StartGame()
	}
	if r.state.Phase == domain.PhasePlaying && r.tickCancel == nil {
		r.startTick()
	}
}

// HandleMessage 处理客户端消息
func (r *Room) HandleMessage(playerID string, msgType byte, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.state.Players[playerID]
	if !ok {
		return nil
	}

	// 速率限制
	now := time.Now().UnixMilli()
	if now-player.MessageWindowStart > int64(config.MessageWindowMs) {
		player.MessageCount = 0
		player.MessageWindowStart = now
	}
	player.MessageCount++
	if player.MessageCount > protocol.MessageRateLimit {
		if pc, ok := r.connections[playerID]; ok {
			pc.Conn.Close()
			delete(r.connections, playerID)
		}
		return nil
	}

	switch msgType {
	case protocol.MsgTap:
		r.handleTap(player, playerID, payload)
	case protocol.MsgSetNickname:
		r.handleSetNicknameMsg(player, payload)
	case protocol.MsgRestartVote:
		_ = HandleRestartVote(r, player)
	case protocol.MsgPing:
		r.sendToPlayer(playerID, protocol.EncodePong())
	}
	return nil
}

// HandleDisconnect 处理玩家断连
func (r *Room) HandleDisconnect(playerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.state.Players[playerID]
	if !ok {
		return nil
	}

	// 删除连接
	delete(r.connections, playerID)

	// P3-1.3：断连标记迁移到领域对象方法
	now := time.Now().UnixMilli()
	player.MarkDisconnected(now)
	r.logger.Info("player disconnected, grace period 30s", "playerID", playerID)

	return nil
}

// StartGame 开始游戏（waiting → countdown → playing）
func (r *Room) StartGame() error {
	if r.state.Phase != domain.PhaseWaiting {
		return nil
	}

	r.state.Phase = domain.PhaseCountdown
	r.countdownStart = time.Now().UnixMilli()
	r.state.RestartVotes = make(map[string]bool)
	r.state.RestartTimerStart = nil
	r.state.StartedAt = time.Now().UnixMilli()
	r.state.SessionID = idgen.UUID()

	ResetGameEntities(r.state, RandomSpawnTimer())

	countdownMs := int64(protocol.CountdownTicks) * 1000 / int64(protocol.TickRate)
	r.scheduleCountdownEnd(time.Now().Add(time.Duration(countdownMs) * time.Millisecond))
	r.broadcastCritical(protocol.EncodeGameStateChange(protocol.PhaseCountdown))
	r.broadcast(r.buildSnapshot(), "")

	r.logger.Info("startGame", "phase", "countdown", "countdownMs", countdownMs)
	r.saveState()
	return nil
}

// EndGame 结束游戏（playing → ended）
func (r *Room) EndGame() error {
	r.state.Phase = domain.PhaseEnded
	r.stopTick()

	// 异步记录游戏结果到数据库（通过 Redis Stream 队列）
	// 企业为何需要：游戏结束热路径不应被 PG 写入延迟阻塞。异步队列削峰填谷，Worker 批量写入提升吞吐。
	if r.hub != nil && r.hub.redis != nil && r.state.SessionID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
		defer cancel()

		endedAt := time.Now().UnixMilli()
		results := make([]map[string]interface{}, 0, len(r.state.Players))
		for _, p := range r.state.Players {
			results = append(results, map[string]interface{}{
				"user_id":            p.ID,
				"score_contribution": p.ScoreContribution,
				"taps_count":         p.TapsCount,
			})
		}

		payload := map[string]interface{}{
			"game_id":     r.state.SessionID,
			"room_code":   r.state.LobbyCode,
			"final_score": r.state.Balloon.Score,
			"results":     results,
			"ended_at":    endedAt,
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			r.logger.Error("marshal game result payload", "error", err)
		} else if err := r.hub.redis.EnqueueGameResult(ctx, payloadJSON); err != nil {
			r.logger.Error("enqueue game result", "error", err)
		}
	}

	r.broadcastCritical(protocol.EncodeGameStateChange(protocol.PhaseEnded))
	r.saveState()

	// 设置自动重启定时器
	if len(r.connections) > 0 {
		r.scheduleAutoRestart(time.Now().Add(time.Duration(protocol.AutoRestartMs) * time.Millisecond))
	} else {
		r.state.Phase = domain.PhaseWaiting
		r.logger.Info("no players, phase reset to waiting")
	}

	return nil
}

// tick 是 15Hz 的 tick 循环
func (r *Room) tick(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(int64(1000/protocol.TickRate)) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			r.tickOnce()
			r.mu.Unlock()
		}
	}
}

// tickOnce 执行一次 tick 逻辑
func (r *Room) tickOnce() {
	if r.state.Phase != domain.PhasePlaying {
		return
	}

	// 清理断连玩家
	r.cleanupDisconnected(time.Now().UnixMilli())

	// 检查是否有在线玩家
	hasConnected := false
	for _, p := range r.state.Players {
		if !p.Disconnected {
			hasConnected = true
			break
		}
	}
	if !hasConnected && len(r.state.Players) > 0 {
		r.stopTick()
		return
	}

	if len(r.state.Players) == 0 {
		r.stopTick()
		return
	}

	r.state.TickCount++

	// 物理更新
	gameOver := ApplyPhysics(&r.state.Balloon)
	if gameOver {
		r.EndGame()
		return
	}

	UpdateWind(r.state)
	UpdateBirdAI(&r.state.Bird, &r.state.Balloon, r.state.TickCount)
	UpdateGhostAI(r.state)

	if CheckGhostCollision(r.state) {
		_ = r.EndGame()
		return
	}

	if CheckBirdCollision(&r.state.Bird, &r.state.Balloon) {
		_ = r.EndGame()
		return
	}

	r.broadcast(r.buildSnapshot(), "")

	// 每 30 ticks 持久化
	if r.state.TickCount%30 == 0 {
		r.saveState()
	}
}

// startTick 启动 tick 循环
func (r *Room) startTick() {
	if r.tickCancel != nil {
		return
	}
	r.wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	r.tickCancel = cancel
	go func() {
		defer r.wg.Done()
		r.tick(ctx)
	}()
}

// stopTick 停止 tick 循环
func (r *Room) stopTick() {
	if r.tickCancel != nil {
		r.tickCancel()
		r.tickCancel = nil
	}
}

// broadcast 发送数据给所有连接（可排除一个玩家），并发布到 Redis 供跨实例投递。
//
// P4-5: 发送缓冲区满时记录 ws_messages_dropped_total 指标，并跟踪连续丢弃次数。
// 连续 3 次丢弃记录 WARN 日志，连续 10 次丢弃强制断开慢客户端。
// 调用方必须持有 r.mu 锁。
func (r *Room) broadcast(data []byte, excludePlayerID string) {
	r.broadcastLocal(data, excludePlayerID)
	r.publishBroadcast(data, excludePlayerID, false)
}

// broadcastLocal 仅向本地连接投递数据，不发布到 Redis。
// 由 Hub 在收到 Redis Pub/Sub 远程消息时调用，避免回环。
// 调用方必须持有 r.mu 锁。
func (r *Room) broadcastLocal(data []byte, excludePlayerID string) {
	for pid, pc := range r.connections {
		if pid == excludePlayerID {
			continue
		}
		select {
		case pc.Send <- data:
			pc.consecutiveDrops = 0
		default:
			// Channel full — drop message
			metrics.WSMessagesDroppedTotal.WithLabelValues(r.state.LobbyCode).Inc()
			pc.consecutiveDrops++
			drops := pc.consecutiveDrops

			if drops >= 3 {
				slog.Warn("slow client: messages being dropped",
					"user_id", pc.PlayerID,
					"drops", drops,
					"room_code", r.state.LobbyCode)
			}
			if drops >= 10 {
				slog.Warn("disconnecting slow client",
					"user_id", pc.PlayerID,
					"drops", drops,
					"room_code", r.state.LobbyCode)
				// Force disconnect — close the connection.
				// readPump will error out and trigger HandleDisconnect.
				_ = pc.Conn.Close()
				delete(r.connections, pid)
			}
		}
	}
}

// publishBroadcast 将消息发布到 Redis Pub/Sub 供其他实例投递。
// 使用短超时避免阻塞游戏 tick；发布失败仅记录 WARN（本地投递已完成）。
// 调用方必须持有 r.mu 锁。
func (r *Room) publishBroadcast(data []byte, excludePlayerID string, critical bool) {
	if r.broadcaster == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	msg := BroadcastMessage{
		RoomCode:        r.state.LobbyCode,
		ExcludePlayer:   excludePlayerID,
		ExcludeInstance: r.instanceID,
		Payload:         data,
		Critical:        critical,
	}
	if err := r.broadcaster.Publish(ctx, r.state.LobbyCode, msg); err != nil {
		r.logger.Warn("redis publish failed, local-only delivery",
			"error", err,
			"room", r.state.LobbyCode)
	}
}

// broadcastCritical sends a message to all connections with a blocking send
// and timeout. Used for critical phase messages (PhaseEnded, PhaseCountdown)
// that must reach clients even if their buffer is full.
// 同时发布到 Redis（Critical: true）供跨实例投递。
//
// P4-5: 关键消息（阶段转换）不能被静默丢弃，使用带超时的阻塞发送。
// 调用方必须持有 r.mu 锁。
func (r *Room) broadcastCritical(message []byte) {
	for _, pc := range r.connections {
		if pc == nil {
			continue
		}
		select {
		case pc.Send <- message:
			pc.consecutiveDrops = 0
		case <-time.After(100 * time.Millisecond):
			// Critical message timed out — log but don't drop
			slog.Error("critical message send timeout",
				"user_id", pc.PlayerID,
				"room_code", r.state.LobbyCode)
		}
	}
	r.publishBroadcast(message, "", true)
}

// sendToPlayer 发送数据给指定玩家
func (r *Room) sendToPlayer(playerID string, data []byte) {
	if pc, ok := r.connections[playerID]; ok {
		select {
		case pc.Send <- data:
		default:
		}
	}
}

// modelPhaseToProtocol 将 domain.GamePhase 转换为 protocol.GamePhase
func modelPhaseToProtocol(p domain.GamePhase) protocol.GamePhase {
	return protocol.GamePhase(string(p))
}

// buildSnapshot 编码当前状态为快照
func (r *Room) buildSnapshot() []byte {
	// Reuse the players slice (reset length, keep backing array) to avoid
	// allocating a new slice on every snapshot. buildSnapshot is always called
	// under r.mu, so concurrent access is safe.
	players := r.players[:0]
	for _, p := range r.state.Players {
		cooldownRemaining := int64(0)
		now := time.Now().UnixMilli()
		if p.CooldownEndTime > now {
			cooldownRemaining = p.CooldownEndTime - now
		}
		players = append(players, protocol.PlayerState{
			PlayerIndex:       uint16(p.PlayerIndex),       //nolint:gosec // bounded
			CooldownMs:        uint32(cooldownRemaining),   //nolint:gosec // bounded by cooldown duration
			Palette:           uint32(p.Palette),           //nolint:gosec // Palette < 8
			ScoreContribution: uint32(p.ScoreContribution), //nolint:gosec // game score, non-negative
			Nickname:          p.Nickname,
		})
	}
	r.players = players

	return protocol.EncodeSnapshot(
		modelPhaseToProtocol(r.state.Phase),
		uint32(r.state.TickCount), //nolint:gosec // tick counter wraps naturally
		uint32(r.state.Balloon.Score),
		protocol.BalloonState{
			X:  float32(r.state.Balloon.X),
			Y:  float32(r.state.Balloon.Y),
			Vy: float32(r.state.Balloon.VY),
			Vx: float32(r.state.Balloon.VX),
		},
		protocol.BirdState{
			X:      float32(r.state.Bird.X),
			Y:      float32(r.state.Bird.Y),
			Active: r.state.Bird.Active,
		},
		protocol.GhostState{
			X:          float32(r.state.Ghost.X),
			Y:          float32(r.state.Ghost.Y),
			Active:     r.state.Ghost.Active,
			RepelTimer: uint16(r.state.Ghost.RepelTimer), //nolint:gosec // bounded timer
		},
		players,
		nil, // ripples
		r.state.Wind,
	)
}

// saveStateWithError persists state to PostgreSQL and returns any error.
// P4-6.1: 暴露 error 供 Saga 补偿模式判断是否回滚。
func (r *Room) saveStateWithError() error {
	if r.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
	defer cancel()

	data, err := SerializeState(r.state)
	if err != nil {
		return fmt.Errorf("serialize state: %w", err)
	}

	ls := &domain.LobbyState{
		Code:      r.state.LobbyCode,
		State:     string(data),
		UpdatedAt: time.Now().UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := r.store.SaveLobbyState(ctx, ls); err != nil {
		return fmt.Errorf("save lobby state: %w", err)
	}
	return nil
}

// saveState 持久化到 PostgreSQL
func (r *Room) saveState() {
	if err := r.saveStateWithError(); err != nil {
		r.logger.Error("save state", "error", err)
	}
}

// cleanupDisconnected 清理超过 30 秒优雅期的断连玩家
func (r *Room) cleanupDisconnected(now int64) {
	for pid, player := range r.state.Players {
		if player.Disconnected && player.DisconnectedAt != nil && now-*player.DisconnectedAt > protocol.ReconnectGraceMs {
			delete(r.state.Players, pid)
			delete(r.usedNames, player.Nickname)
			delete(r.state.RestartVotes, pid)
			r.logger.Info("removed disconnected player after grace", "playerID", pid)
			r.broadcast(protocol.EncodePlayerLeave(uint16(player.PlayerIndex)), "")
		}
	}
}

// setEndGameAlarm 设置 ended/countdown 阶段的定时器。
// 定时器触发时根据当前阶段分发到 handleCountdownEnd 或 handleAutoRestart。
func (r *Room) setEndGameAlarm(when time.Time) {
	if r.endGameTimer != nil {
		r.endGameTimer.Stop()
	}
	duration := time.Until(when)
	if duration < 0 {
		duration = 0
	}
	r.endGameTimer = time.AfterFunc(duration, func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if r.state.Phase == domain.PhaseCountdown {
			r.handleCountdownEnd()
			return
		}
		if r.state.Phase == domain.PhaseEnded {
			r.handleAutoRestart()
		}
	})
}

// scheduleCountdownEnd 调度倒计时结束定时器（countdown → playing）。
func (r *Room) scheduleCountdownEnd(when time.Time) {
	r.setEndGameAlarm(when)
}

// scheduleAutoRestart 调度自动重启定时器（ended → countdown）。
func (r *Room) scheduleAutoRestart(when time.Time) {
	r.setEndGameAlarm(when)
}

// handleCountdownEnd 处理倒计时结束：转为 playing 阶段并启动 tick。
func (r *Room) handleCountdownEnd() {
	r.state.Phase = domain.PhasePlaying
	r.startTick()
	r.broadcast(protocol.EncodeGameStateChange(protocol.PhasePlaying), "")
	r.broadcast(r.buildSnapshot(), "")

	metrics.GameSessionsTotal.Inc()

	if r.store != nil && r.state.SessionID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
		defer cancel()
		// P4-6.3: Saga 补偿模式 — CreateGameSession 失败时记录告警。
		// 游戏在内存中继续，结果将在 EndGame 时通过 Redis Stream 异步保存。
		if err := r.store.CreateGameSession(ctx, &domain.GameSession{
			ID:        r.state.SessionID,
			LobbyCode: r.state.LobbyCode,
			Status:    "playing",
			StartedAt: &r.state.StartedAt,
		}); err != nil {
			slog.Warn("create game session failed, will retry",
				"error", err,
				"room_code", r.state.LobbyCode)
			// TODO: add to retry queue (could use Redis Stream or outbox)
		}
	}
	r.saveState()
}

// handleAutoRestart 处理 ended 阶段的自动重启逻辑。
func (r *Room) handleAutoRestart() {
	for pid := range r.state.RestartVotes {
		p, ok := r.state.Players[pid]
		if !ok || p.Disconnected {
			delete(r.state.RestartVotes, pid)
		}
	}

	if len(r.connections) == 0 {
		r.state.Phase = domain.PhaseWaiting
		r.logger.Info("phase=ended but no players, phase reset to waiting")
		return
	}

	activeVotes := 0
	for _, v := range r.state.RestartVotes {
		if v {
			activeVotes++
		}
	}
	if activeVotes > 0 {
		r.logger.Info("phase=ended but restart votes active, deferring auto-restart by 30s")
		r.scheduleAutoRestart(time.Now().Add(time.Duration(protocol.RestartTimeoutMs) * time.Millisecond))
		return
	}

	r.logger.Info("phase=ended, no active votes, auto-restarting")
	_ = RestartAndStart(r)
}

// handleTap 处理点击
func (r *Room) handleTap(player *domain.PlayerState, playerID string, payload []byte) {
	now := time.Now().UnixMilli()

	if !r.validateTapRequest(player, now) {
		r.sendToPlayer(playerID, protocol.EncodeTapRejected())
		return
	}

	tapX, tapY, ok := r.decodeTapPayload(payload)
	if !ok {
		r.sendToPlayer(playerID, protocol.EncodeTapRejected())
		return
	}

	if !r.applyTapPhysics(float64(tapX), float64(tapY)) {
		r.sendToPlayer(playerID, protocol.EncodeTapRejected())
		return
	}

	cooldown := r.updatePlayerStats(player, now)
	r.broadcastTapResult(player, cooldown)
}

// validateTapRequest 校验点击请求的阶段与冷却时间。
func (r *Room) validateTapRequest(player *domain.PlayerState, now int64) bool {
	if r.state.Phase != domain.PhasePlaying {
		return false
	}
	// P3-1.3：冷却判断迁移到领域对象方法
	if !player.CanTap(now) {
		return false
	}
	return true
}

// decodeTapPayload 解码并校验点击坐标的合法性与范围。
func (r *Room) decodeTapPayload(payload []byte) (float32, float32, bool) {
	if len(payload) < 8 {
		return 0, 0, false
	}
	tapX, tapY, ok := protocol.DecodeTap(payload)
	if !ok {
		return 0, 0, false
	}
	if math.IsNaN(float64(tapX)) || math.IsNaN(float64(tapY)) ||
		math.IsInf(float64(tapX), 0) || math.IsInf(float64(tapY), 0) ||
		float64(tapX) < 0 || float64(tapX) > 1 || float64(tapY) < 0 || float64(tapY) > 1 {
		return 0, 0, false
	}
	return tapX, tapY, true
}

// applyTapPhysics 应用力与幽灵排斥，返回是否成功。
func (r *Room) applyTapPhysics(tapX, tapY float64) bool {
	if !ApplyTapForce(&r.state.Balloon, tapX, tapY) {
		return false
	}
	ApplyGhostRepel(r.state, tapX, tapY)
	return true
}

// updatePlayerStats 更新玩家点击统计与冷却时间，返回冷却时长。
func (r *Room) updatePlayerStats(player *domain.PlayerState, now int64) int64 {
	connectedCount := 0
	for _, p := range r.state.Players {
		if !p.Disconnected {
			connectedCount++
		}
	}
	cooldown := CalculateCooldown(connectedCount)
	// P3-1.3：点击统计与冷却更新迁移到领域对象方法
	player.RecordTap(now, cooldown)
	r.state.Balloon.Score++
	return cooldown
}

// broadcastTapResult 广播点击接受消息给所有玩家。
func (r *Room) broadcastTapResult(player *domain.PlayerState, cooldown int64) {
	tapMsg := protocol.EncodeTapAccepted(
		uint16(player.PlayerIndex),
		uint32(cooldown), //nolint:gosec // bounded by cooldown duration
		float32(r.state.Balloon.X),
		float32(r.state.Balloon.Y),
	)
	r.broadcast(tapMsg, "")
}

// handleSetNicknameMsg 处理设置昵称消息
func (r *Room) handleSetNicknameMsg(player *domain.PlayerState, payload []byte) {
	if len(payload) < 1 {
		return
	}
	nickLen := int(payload[0])
	if nickLen > config.MaxNicknameLen {
		return
	}
	if len(payload) < 1+nickLen {
		return
	}
	nickname := string(payload[1 : 1+nickLen])

	if HandleSetNickname(r.state, player, nickname, r.usedNames) {
		r.broadcast(r.buildSnapshot(), "")
		r.saveState()
	}
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

	// Wait for tick goroutine to exit outside r.mu to avoid deadlock
	// (tick goroutine may be waiting for r.mu in the ticker.C branch).
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

	// Persist final state so the room can be restored on restart.
	r.saveState()
}

// ErrRoomFull 房间玩家已满
var ErrRoomFull = &roomFullError{}

type roomFullError struct{}

func (e *roomFullError) Error() string { return "room is full" }
