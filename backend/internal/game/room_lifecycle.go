package game

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
)

// HandleJoin 处理玩家加入/重连
func (r *Room) HandleJoin(playerID string, conn *websocket.Conn) error {
	start := time.Now()
	r.mu.Lock()
	defer func() {
		recordRoomLock("join", start)
		r.mu.Unlock()
	}()

	player := r.state.Players[playerID]

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

	r.notifyJoin(playerID, player, false)
	r.normalizePhaseForNicknameGate()
	r.transitionPhaseIfNeeded()

	return nil
}

// closeExistingConnection 关闭玩家已有连接（重连时替换旧连接）。
func (r *Room) closeExistingConnection(playerID string, player *domain.PlayerState) {
	if player == nil || player.Disconnected {
		return
	}
	if _, ok := r.connections[playerID]; ok {
		r.logger.Info("closing old WebSocket for player", "playerID", playerID)
		r.removeConnectionLocked(playerID)
	}
}

// reconnectPlayer 处理断连重连：恢复状态、发送快照、重置定时器。
func (r *Room) reconnectPlayer(playerID string, player *domain.PlayerState) {
	player.Disconnected = false
	player.DisconnectedAt = nil
	r.logger.Info("player reconnected during grace period", "playerID", playerID)
	r.sendToPlayer(playerID, r.buildSnapshot())
	r.saveState()

	switch r.state.Phase {
	case domain.PhaseWaiting:
		r.tryStartWhenAllReady()
	case domain.PhasePlaying:
		if r.tickCancel == nil {
			r.startTick()
		}
	case domain.PhaseCountdown:
		if r.tickCancel == nil {
			r.resumeCountdownForReconnect(playerID)
		}
	}
}

func (r *Room) resumeCountdownForReconnect(playerID string) {
	remaining := remainingCountdownMs(r.countdownStart)
	r.sendToPlayer(playerID, protocol.EncodeGameStateChange(protocol.PhaseCountdown, uint32(remaining))) //nolint:gosec:G115 // bounded countdown
	r.setEndGameAlarm(time.Now().Add(time.Duration(remaining) * time.Millisecond))
}

// addNewPlayer 添加新玩家，房间满时返回 ErrRoomFull。
func (r *Room) addNewPlayer(playerID string, conn *websocket.Conn) (*domain.PlayerState, error) {
	if len(r.state.Players) >= r.maxPlayers {
		delete(r.connections, playerID)
		if conn != nil {
			_ = conn.Close()
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

	joinMsg := protocol.EncodePlayerJoin(uint16(player.PlayerIndex), player.Nickname, uint32(player.Palette)) //nolint:gosec:G115 // PlayerIndex < MaxPlayersPerRoom, Palette < 8
	r.broadcast(joinMsg, playerID)

	r.saveState()
}

// normalizePhaseForNicknameGate 若有玩家未确认昵称，不应处于 countdown/playing。
func (r *Room) normalizePhaseForNicknameGate() {
	if r.state.Phase != domain.PhaseCountdown && r.state.Phase != domain.PhasePlaying {
		return
	}
	if r.allConnectedPlayersReady() {
		return
	}
	if r.state.Phase == domain.PhasePlaying {
		r.stopTick()
	}
	if r.endGameTimer != nil {
		r.endGameTimer.Stop()
		r.endGameTimer = nil
	}
	if r.startDelayTimer != nil {
		r.startDelayTimer.Stop()
		r.startDelayTimer = nil
	}
	r.state.Phase = domain.PhaseWaiting
	r.logger.Info("phase reset to waiting: not all players confirmed nickname")
}

// transitionPhaseIfNeeded 检查并执行阶段转换（恢复 tick）。
func (r *Room) transitionPhaseIfNeeded() {
	if r.state.Phase == domain.PhasePlaying && r.tickCancel == nil {
		r.startTick()
	}
}

// allConnectedPlayersReady 检查所有已连接玩家是否均已确认昵称。
func (r *Room) allConnectedPlayersReady() bool {
	if len(r.connections) == 0 {
		return false
	}
	for pid := range r.connections {
		player, ok := r.state.Players[pid]
		if !ok || player.Disconnected || !player.NicknameConfirmed {
			return false
		}
	}
	return true
}

// tryStartWhenAllReady 当所有已连接玩家确认昵称后，从 waiting 进入 countdown。
// 延迟 1.5 秒启动，给玩家时间看到欢迎信息。
func (r *Room) tryStartWhenAllReady() {
	if r.state.Phase != domain.PhaseWaiting {
		return
	}
	if !r.allConnectedPlayersReady() {
		return
	}
	if r.startDelayTimer != nil {
		return // 已在等待启动
	}
	r.startDelayTimer = time.AfterFunc(r.startDelay, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.startDelayTimer = nil
		if r.state.Phase == domain.PhaseWaiting && r.allConnectedPlayersReady() {
			_ = r.StartGame()
		}
	})
}

// HandleDisconnect 处理玩家断开连接
func (r *Room) HandleDisconnect(playerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.state.Players[playerID]
	if !ok {
		return nil
	}

	delete(r.connections, playerID)

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

	ResetGameEntities(r.state, RandomSpawnTimer(r.rng), r.rng)

	r.scheduleCountdownFromNow()
	r.broadcastCountdownPhase()

	r.logger.Info("startGame", "phase", "countdown", "countdownMs", countdownDurationMs())
	r.saveState()
	return nil
}

// EndGame 结束游戏（playing → ended）
func (r *Room) EndGame() error {
	return r.EndGameWithReason(protocol.EndReasonNone)
}

// EndGameWithReason ends the game and broadcasts the death reason to clients.
func (r *Room) EndGameWithReason(endReason uint8) error {
	r.state.Phase = domain.PhaseEnded
	r.stopTick()

	if r.state.Balloon.Y < 0 {
		r.state.Balloon.Y = 0
	}

	r.enqueueGameResultAsync()
	r.broadcastGameEnded(endReason)

	if len(r.connections) > 0 {
		r.setEndGameAlarm(time.Now().Add(time.Duration(domain.AutoRestartMs) * time.Millisecond))
	} else {
		r.state.Phase = domain.PhaseWaiting
		r.logger.Info("no players, phase reset to waiting")
	}

	return nil
}

func (r *Room) broadcastGameEnded(endReason uint8) {
	r.broadcast(r.buildSnapshot(), "")
	r.broadcastCritical(protocol.EncodeGameStateChangeEnded(endReason))
	r.saveState()
}

// setEndGameAlarm 设置 ended/countdown 阶段的闹钟定时器。
func (r *Room) setEndGameAlarm(when time.Time) {
	if r.endGameTimer != nil {
		r.endGameTimer.Stop()
	}
	duration := time.Until(when)
	if duration < 0 {
		duration = 0
	}
	r.endGameAlarmVersion++
	capturedVersion := r.endGameAlarmVersion
	r.endGameTimer = time.AfterFunc(duration, func() {
		r.mu.Lock()
		phase := r.state.Phase
		version := r.endGameAlarmVersion
		r.mu.Unlock()

		if capturedVersion != version {
			return
		}

		switch phase {
		case domain.PhaseCountdown:
			r.handleCountdownEnd()
		case domain.PhaseEnded:
			r.mu.Lock()
			defer r.mu.Unlock()
			r.handleAutoRestart()
		}
	})
}

// handleCountdownEnd 处理倒计时结束：转为 playing 阶段并启动 tick。
func (r *Room) handleCountdownEnd() {
	r.mu.Lock()
	if r.state.Phase != domain.PhaseCountdown {
		r.mu.Unlock()
		return
	}
	r.state.Phase = domain.PhasePlaying
	r.state.TickCount++
	r.broadcastCritical(protocol.EncodeGameStateChange(protocol.PhasePlaying))
	r.broadcast(r.buildSnapshot(), "")

	metrics.GameSessionsTotal.Inc()

	if r.store != nil && r.state.SessionID != "" {
		r.createGameSessionAsync(&domain.GameSession{
			ID:        r.state.SessionID,
			LobbyCode: r.state.LobbyCode,
			Status:    "active", // DB CHECK 约束只允许 'active' 或 'ended'
			StartedAt: &r.state.StartedAt,
		})
	}
	r.saveState()
	r.mu.Unlock()

	r.restartTick()
}

// handleAutoRestart ended 阶段自动重启。
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

	yesVotes, _ := countRestartYesVotes(r.state.Players, r.state.RestartVotes)
	if yesVotes > 0 {
		r.logger.Info("phase=ended but restart votes active, deferring auto-restart by 30s")
		r.setEndGameAlarm(time.Now().Add(time.Duration(domain.RestartTimeoutMs) * time.Millisecond))
		return
	}

	r.logger.Info("phase=ended, no active votes, auto-restarting")
	_ = RestartAndStart(r)
}
