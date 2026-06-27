package game

import (
	"context"
	"math"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
)

// HandleMessage 处理客户端消息
func (r *Room) HandleMessage(playerID string, msgType byte, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.state.Players[playerID]
	if !ok {
		return nil
	}

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

	r.cleanupDisconnected(time.Now().UnixMilli())

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

	gameOver := ApplyPhysics(&r.state.Balloon)
	if gameOver {
		r.EndGame()
		return
	}

	UpdateWind(r.state)
	UpdateBirdAI(&r.state.Bird, &r.state.Balloon, r.state.TickCount)
	UpdateGhostAI(r.state)
	if CheckGhostCollision(r.state) {
		r.EndGame()
		return
	}

	if CheckBirdCollision(&r.state.Bird, &r.state.Balloon) {
		_ = r.EndGame()
		return
	}

	r.broadcast(r.buildSnapshot(), "")

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
		metrics.NicknameConfirmTotal.WithLabelValues("rejected").Inc()
		return
	}
	nickLen := int(payload[0])
	// Length byte is UTF-8 byte count (wire format), not rune count; CJK nicknames can exceed MaxNicknameLen bytes.
	if nickLen <= 0 || nickLen > 255 {
		metrics.NicknameConfirmTotal.WithLabelValues("rejected").Inc()
		return
	}
	if len(payload) < 1+nickLen {
		metrics.NicknameConfirmTotal.WithLabelValues("rejected").Inc()
		return
	}
	nickname := string(payload[1 : 1+nickLen])
	sanitized := sanitizeNickname(nickname)
	if sanitized == "" {
		metrics.NicknameConfirmTotal.WithLabelValues("rejected").Inc()
		return
	}

	player.NicknameConfirmed = true
	metrics.NicknameConfirmTotal.WithLabelValues("accepted").Inc()

	now := time.Now().UnixMilli()
	if player.LastNicknameChange != 0 && now-player.LastNicknameChange < protocol.NicknameCooldownMs {
		r.broadcast(r.buildSnapshot(), "")
		r.tryStartWhenAllReady()
		return
	}

	if HandleSetNickname(r.state, player, sanitized, r.usedNames) {
		r.saveState()
	}
	r.broadcast(r.buildSnapshot(), "")
	r.tryStartWhenAllReady()
}
