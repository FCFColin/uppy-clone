package game

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── Join / Disconnect ───────────────────────────────────────────────

// HandleJoin 处理玩家加入/重连
func (r *Room) HandleJoin(playerID string, conn *websocket.Conn) error {
	start := time.Now()
	r.mu.Lock()
	defer func() {
		metrics.RecordRoomLockHold("join", time.Since(start))
		r.mu.Unlock()
	}()

	player := r.state.Players[playerID]

	r.closeExistingConnection(playerID, player)

	pc := &PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Send:     make(chan []byte, config.WSChannelBuffer),
	}
	r.connMu.Lock()
	r.connections[playerID] = pc
	r.connMu.Unlock()

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

func (r *Room) closeExistingConnection(playerID string, player *domain.PlayerState) {
	if player == nil || player.Disconnected {
		return
	}
	if _, ok := r.connections[playerID]; ok {
		r.logger.Info("closing old WebSocket for player", "playerID", playerID)
		r.removeConnectionLocked(playerID)
	}
}

func (r *Room) reconnectPlayer(playerID string, player *domain.PlayerState) {
	player.Disconnected = false
	player.DisconnectedAt = nil
	r.logger.Info("player reconnected during grace period", "playerID", playerID)
	r.sendToPlayer(playerID, r.buildSnapshot())
	r.requestPersist()

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
	case domain.PhaseEnded:
		r.triggerAutoRestartIfEnded()
	}
}

func (r *Room) resumeCountdownForReconnect(playerID string) {
	remaining := remainingCountdownMs(r.countdownStart)
	r.sendToPlayer(playerID, protocol.EncodeGameStateChange(protocol.PhaseCountdown, uint32(remaining))) //nolint:gosec // G115: bounded countdown
	r.setEndGameAlarm(time.Now().Add(time.Duration(remaining) * time.Millisecond))
}

func (r *Room) addNewPlayer(playerID string, conn *websocket.Conn) (*domain.PlayerState, error) {
	if len(r.state.Players) >= r.maxPlayers {
		r.connMu.Lock()
		delete(r.connections, playerID)
		r.connMu.Unlock()
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

func (r *Room) notifyJoin(playerID string, player *domain.PlayerState, isReconnect bool) {
	if isReconnect {
		r.logger.Info("player reconnect", "playerID", playerID, "index", player.PlayerIndex)
	} else {
		r.logger.Info("player join", "playerID", playerID, "index", player.PlayerIndex)
	}

	r.sendToPlayer(playerID, r.buildSnapshot())

	joinMsg := protocol.EncodePlayerJoin(uint16(player.PlayerIndex), player.Nickname, uint32(player.Palette)) //nolint:gosec // G115: PlayerIndex < MaxPlayersPerRoom, Palette < 10
	r.broadcast(joinMsg, playerID)

	r.requestPersist()
}

// HandleDisconnect 处理玩家断开连接
func (r *Room) HandleDisconnect(playerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.state.Players[playerID]
	if !ok {
		return nil
	}

	r.connMu.Lock()
	delete(r.connections, playerID)
	r.connMu.Unlock()

	now := time.Now().UnixMilli()
	player.MarkDisconnected(now)
	r.logger.Info("player disconnected, grace period 30s", "playerID", playerID)

	return nil
}

// ─── Phase Transition / Start Game ───────────────────────────────────

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

func (r *Room) transitionPhaseIfNeeded() {
	if r.state.Phase == domain.PhasePlaying && r.tickCancel == nil {
		r.startTick()
	}
}

func (r *Room) allConnectedPlayersReady() bool {
	r.connMu.RLock()
	conns := make([]string, 0, len(r.connections))
	for pid := range r.connections {
		conns = append(conns, pid)
	}
	r.connMu.RUnlock()
	if len(conns) == 0 {
		return false
	}
	for _, pid := range conns {
		player, ok := r.state.Players[pid]
		if !ok || player.Disconnected || !player.NicknameConfirmed {
			return false
		}
	}
	return true
}

func (r *Room) tryStartWhenAllReady() {
	if r.state.Phase != domain.PhaseWaiting {
		return
	}
	if !r.allConnectedPlayersReady() {
		return
	}
	if r.startDelayTimer != nil {
		return
	}
	r.startDelayTimer = time.AfterFunc(r.startDelay, func() {
		if r.closed.Load() {
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.closed.Load() {
			return
		}
		r.startDelayTimer = nil
		if r.state.Phase == domain.PhaseWaiting && r.allConnectedPlayersReady() {
			_ = r.StartGame()
		}
	})
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
	r.state.SessionID = domain.UUID()

	ResetGameEntities(r.state, RandomSpawnTimer(r.rng), r.rng)

	r.scheduleCountdownFromNow()
	r.broadcastCountdownPhase()

	r.logger.Info("startGame", "phase", "countdown", "countdownMs", countdownDurationMs())
	r.requestPersist()
	return nil
}

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
			LobbyCode: string(r.state.LobbyCode),
			Status:    "active",
			StartedAt: &r.state.StartedAt,
		})
	}
	r.requestPersist()
	r.mu.Unlock()

	r.restartTick()
}

// ─── Countdown Helpers ───────────────────────────────────────────────

func countdownDurationMs() int64 {
	if protocol.TickRate == 0 {
		return 3000
	}
	return int64(protocol.CountdownTicks) * 1000 / int64(protocol.TickRate)
}

func countdownDurationMsU32() uint32 {
	if protocol.TickRate == 0 {
		return 3000
	}
	return uint32(protocol.CountdownTicks) * 1000 / uint32(protocol.TickRate)
}

func (r *Room) scheduleCountdownFromNow() {
	ms := countdownDurationMs()
	r.setEndGameAlarm(time.Now().Add(time.Duration(ms) * time.Millisecond))
}

func (r *Room) broadcastCountdownPhase() {
	msU32 := countdownDurationMsU32()
	r.broadcastCritical(protocol.EncodeGameStateChange(protocol.PhaseCountdown, msU32))
	r.broadcast(r.buildSnapshot(), "")
}

func remainingCountdownMs(countdownStart int64) int64 {
	elapsed := time.Now().UnixMilli() - countdownStart
	remaining := countdownDurationMs() - elapsed
	if remaining < 100 {
		return 100
	}
	return remaining
}

// ─── End Game ────────────────────────────────────────────────────────

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

	r.broadcastGameEnded(endReason)
	r.recordGameResultAsync()

	r.connMu.RLock()
	hasConns := len(r.connections) > 0
	r.connMu.RUnlock()
	if hasConns {
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
	r.requestPersist()
}

// recordGameResultAsync persists game results to the database without blocking.
func (r *Room) recordGameResultAsync() {
	if r.store == nil || r.state.SessionID == "" {
		return
	}
	sessionID := r.state.SessionID
	roomCode := string(r.state.LobbyCode)
	endedAt := time.Now().UnixMilli()
	finalScore := r.state.Balloon.Score

	var results []domain.GameResultPlayer
	for _, p := range r.state.Players {
		results = append(results, domain.GameResultPlayer{
			UserID:            p.ID,
			Nickname:          p.Nickname,
			ScoreContribution: int(p.ScoreContribution),
			TapsCount:         int(p.TapsCount),
		})
	}

	r.asyncWg.Add(1)
	go func() {
		defer r.asyncWg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
		defer cancel()
		if err := r.store.RecordGameResult(ctx, sessionID, roomCode, endedAt, finalScore, results); err != nil {
			r.logger.Warn("record game result failed", "error", err, "room_code", roomCode)
		}
	}()
}

// setEndGameAlarm sets a timer for ended/countdown phase transitions.
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
		if r.closed.Load() {
			return
		}
		r.mu.Lock()
		phase := r.state.Phase
		version := r.endGameAlarmVersion
		r.mu.Unlock()

		if capturedVersion != version || r.closed.Load() {
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

// ─── Auto-Restart ────────────────────────────────────────────────────

func (r *Room) handleAutoRestart() {
	toDelete := make([]string, 0, len(r.state.RestartVotes))
	for pid := range r.state.RestartVotes {
		p, ok := r.state.Players[pid]
		if !ok || p.Disconnected {
			toDelete = append(toDelete, pid)
		}
	}
	for _, pid := range toDelete {
		delete(r.state.RestartVotes, pid)
	}

	r.connMu.RLock()
	noConns := len(r.connections) == 0
	r.connMu.RUnlock()
	if noConns {
		r.state.Phase = domain.PhaseWaiting
		r.logger.Info("phase=ended but no players, phase reset to waiting")
		return
	}

	yesVotes, connectedCount := countRestartYesVotes(r.state.Players, r.state.RestartVotes)
	// 共识未达成（有投票但未全员同意）→ defer 30s；共识达成或无投票 → 直接重启。
	// 这与 CheckRestartConsensus 的判断一致，避免单人房间残留投票导致无限 defer。
	if yesVotes > 0 && yesVotes < connectedCount {
		r.logger.Info("phase=ended but restart votes active, deferring auto-restart by 30s",
			"yesVotes", yesVotes, "connectedCount", connectedCount)
		r.setEndGameAlarm(time.Now().Add(time.Duration(domain.RestartTimeoutMs) * time.Millisecond))
		return
	}

	r.logger.Info("phase=ended, auto-restarting (consensus reached or no votes)",
		"yesVotes", yesVotes, "connectedCount", connectedCount)
	_ = RestartAndStart(r)
}
