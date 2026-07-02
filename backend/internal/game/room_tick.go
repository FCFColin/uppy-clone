package game

import (
	"context"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// HandleMessage 处理客户端消息
func (r *Room) HandleMessage(playerID string, msgType byte, payload []byte) error {
	start := time.Now()
	r.mu.Lock()
	defer func() {
		recordRoomLock("message", start)
		r.mu.Unlock()
	}()

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
		r.removeConnectionLocked(playerID)
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
			start := time.Now()
			r.mu.Lock()
			r.tickOnce()
			recordRoomLock("tick", start)
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

	if !hasAnyConnectedPlayer(r.state.Players) && len(r.state.Players) > 0 {
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
		if err := r.EndGameWithReason(protocol.EndReasonGround); err != nil {
			r.logger.Warn("failed to end game on ground collision", "error", err)
		}
		return
	}

	UpdateWind(r.state)
	UpdateBirdAI(&r.state.Bird, &r.state.Balloon, r.state.TickCount)
	UpdateGhostAI(r.state)
	if CheckGhostCollision(r.state) {
		if err := r.EndGameWithReason(protocol.EndReasonGhost); err != nil {
			r.logger.Warn("failed to end game on ghost collision", "error", err)
		}
		return
	}

	if CheckBirdCollision(&r.state.Bird, &r.state.Balloon) {
		if err := r.EndGameWithReason(protocol.EndReasonBird); err != nil {
			r.logger.Warn("failed to end game on bird collision", "error", err)
		}
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
		if player.Disconnected && player.DisconnectedAt != nil && reconnectGraceExpired(*player.DisconnectedAt, now) {
			delete(r.state.Players, pid)
			delete(r.usedNames, player.Nickname)
			delete(r.state.RestartVotes, pid)
			r.logger.Info("removed disconnected player after grace", "playerID", pid)
			r.broadcast(protocol.EncodePlayerLeave(uint16(player.PlayerIndex)), "")
		}
	}
}
