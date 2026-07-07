package game

import (
	"context"
	"errors"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
	"go.opentelemetry.io/otel/attribute"
)

// ErrRateLimited 玩家因消息频率过高被断开
var ErrRateLimited = errors.New("player rate limited")

// HandleMessage 处理客户端消息
func (r *Room) HandleMessage(playerID string, msgType byte, payload []byte) error {
	_, span := tracer.Start(context.Background(), "game.handle_message")
	defer span.End()
	span.SetAttributes(
		attribute.String("player.id", playerID),
		attribute.Int("msg_type", int(msgType)),
	)
	start := time.Now()
	r.mu.Lock()
	defer func() {
		metrics.RecordRoomLockHold("message", time.Since(start))
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
	if player.MessageCount > domain.MessageRateLimit {
		player.MarkDisconnected(now)
		r.removeConnectionLocked(playerID)
		return ErrRateLimited
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
			_, span := tracer.Start(ctx, "game.tick_iteration")
			now := time.Now()

			r.mu.Lock()
			shouldBroadcast := r.tickOnceLocked(now)
			var snapshot []byte
			if shouldBroadcast {
				snapshot = r.buildSnapshot()
			}
			tickCount := r.state.TickCount
			r.mu.Unlock()

			if shouldBroadcast {
				r.broadcast(snapshot, "")
			}

			metrics.RecordGameTickDuration(time.Since(now))
			span.End()

			if tickCount > 0 && tickCount%30 == 0 {
				r.asyncSaveState()
			}
		}
	}
}

// tickOnceLocked 执行一次 tick 的状态变更逻辑（不含 broadcast）。
// 返回 true 表示调用方应构建快照并广播（正常 tick 路径）。
// 返回 false 表示 tick 提前结束（非 playing 阶段、无连接、或碰撞结束游戏）。
// 调用方须持有 r.mu。
func (r *Room) tickOnceLocked(now time.Time) bool {
	nowMs := now.UnixMilli()
	r.cleanupDisconnected(nowMs)

	if r.state.Phase != domain.PhasePlaying {
		return false
	}

	if len(r.state.Players) == 0 || !anyPlayerConnected(r.state.Players) {
		r.stopTick()
		return false
	}

	r.state.TickCount++

	if ApplyPhysics(&r.state.Balloon) {
		r.endGameIf(protocol.EndReasonGround, "ground collision")
		return false
	}

	UpdateWind(r.state, r.rng)
	UpdateBirdAI(&r.state.Bird, &r.state.Balloon, r.state.TickCount, r.rng)
	UpdateGhostAI(r.state, r.rng)
	if CheckGhostCollision(r.state) {
		r.endGameIf(protocol.EndReasonGhost, "ghost collision")
		return false
	}

	if CheckBirdCollision(&r.state.Bird, &r.state.Balloon) {
		r.endGameIf(protocol.EndReasonBird, "bird collision")
		return false
	}

	return true
}

// tickOnce 执行一次完整的 tick 逻辑（含 broadcast）。仅用于测试兼容。
func (r *Room) tickOnce(now time.Time) {
	_, span := tracer.Start(context.Background(), "game.tick_once")
	defer span.End()
	if r.tickOnceLocked(now) {
		r.broadcast(r.buildSnapshot(), "")
	}
}

func (r *Room) endGameIf(reason uint8, logLabel string) {
	if err := r.EndGameWithReason(reason); err != nil {
		r.logger.Warn("failed to end game on "+logLabel, "error", err)
	}
}

// startTickGoroutine launches a tick goroutine (caller must hold r.mu).
func (r *Room) startTickGoroutine() {
	r.wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	r.tickCancel = cancel
	go func() {
		defer r.wg.Done()
		r.tick(ctx)
	}()
}

// startTick 启动 tick 循环
func (r *Room) startTick() {
	if r.tickCancel != nil {
		return
	}
	r.startTickGoroutine()
}

// stopTick 停止 tick 循环。调用方须持有 r.mu。
func (r *Room) stopTick() {
	if r.tickCancel != nil {
		r.tickCancel()
		r.tickCancel = nil
	}
}

// restartTick 停止旧 tick 并等待其退出，然后启动新 tick。
// 调用方不可持有 r.mu。
func (r *Room) restartTick() {
	r.mu.Lock()
	if r.tickCancel == nil {
		r.startTickGoroutine()
		r.mu.Unlock()
		return
	}
	oldCancel := r.tickCancel
	r.tickCancel = nil
	r.mu.Unlock()

	oldCancel()
	r.wg.Wait()

	r.mu.Lock()
	r.startTickGoroutine()
	r.mu.Unlock()
}

// cleanupDisconnected 清理超过 30 秒优雅期的断连玩家
func (r *Room) cleanupDisconnected(now int64) {
	for pid, player := range r.state.Players {
		if player.Disconnected && player.DisconnectedAt != nil && now-*player.DisconnectedAt > domain.ReconnectGraceMs {
			delete(r.state.Players, pid)
			delete(r.usedNames, player.Nickname)
			delete(r.state.RestartVotes, pid)
			r.logger.Info("removed disconnected player after grace", "playerID", pid)
			r.broadcast(protocol.EncodePlayerLeave(uint16(player.PlayerIndex)), "")
		}
	}
}
