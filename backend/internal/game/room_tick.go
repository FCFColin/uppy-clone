package game

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/validate"
	"go.opentelemetry.io/otel/attribute"
)

const persistIntervalTicks = 30

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

	if err := r.enforceRateLimit(player, playerID); err != nil {
		return err
	}

	r.dispatchMessage(player, playerID, msgType, payload)
	return nil
}

// enforceRateLimit tracks per-player message frequency and disconnects players
// exceeding MessageRateLimit within the rolling MessageWindowMs window.
// Caller must hold r.mu.
func (r *Room) enforceRateLimit(player *domain.PlayerState, playerID string) error {
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
	return nil
}

// dispatchMessage routes a decoded message to the appropriate type-specific
// handler. Caller must hold r.mu.
func (r *Room) dispatchMessage(player *domain.PlayerState, playerID string, msgType byte, payload []byte) {
	switch msgType {
	case protocol.MsgTap:
		r.handleTap(player, playerID, payload)
	case protocol.MsgSetNickname:
		r.handleSetNicknameMsg(player, payload)
	case protocol.MsgRestartVote:
		r.handleRestartVoteMsg(player, playerID)
	case protocol.MsgPing:
		r.handlePingMsg(playerID)
	}
}

// tick is the 15Hz game tick loop.
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
			var sd snapshotData
			if shouldBroadcast {
				sd = r.extractSnapshotDataLocked()
			}
			tickCount := r.state.TickCount
			r.mu.Unlock()

			if shouldBroadcast {
				snapshot := encodeSnapshot(sd)
				r.broadcast(snapshot, "")
			}

			metrics.RecordGameTickDuration(time.Since(now))
			span.End()

			if tickCount > 0 && tickCount%persistIntervalTicks == 0 {
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

	r.connMu.RLock()
	noConns := len(r.connections) == 0
	r.connMu.RUnlock()
	if len(r.state.Players) == 0 || noConns {
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
	if r.tickCancel != nil {
		// 另一个 goroutine 已经启动了新 tick
		r.mu.Unlock()
		return
	}
	r.startTickGoroutine()
	r.mu.Unlock()
}

// cleanupDisconnected removes players whose reconnect grace period has expired (30s)
func (r *Room) cleanupDisconnected(now int64) {
	for pid, player := range r.state.Players {
		if player.Disconnected && player.DisconnectedAt != nil && now-*player.DisconnectedAt > domain.ReconnectGraceMs {
			delete(r.state.Players, pid)
			delete(r.usedNames, player.Nickname)
			delete(r.state.RestartVotes, pid)
			r.logger.Info("removed disconnected player after grace", "playerID", pid)
			r.broadcast(protocol.EncodePlayerLeave(uint16(player.PlayerIndex)), "") //nolint:gosec // G115: PlayerIndex < MaxPlayersPerRoom(50)
		}
	}
}

// ─── Message Handlers ────────────────────────────────────────────────

func (r *Room) handleTap(player *domain.PlayerState, playerID string, payload []byte) {
	now := time.Now().UnixMilli()

	if !r.validateTapRequest(player, now) {
		r.sendToPlayer(playerID, protocol.EncodeTapRejected())
		return
	}
	tapX, tapY, ok := r.decodeTapPayload(payload)
	if !ok || !r.applyTapPhysics(float64(tapX), float64(tapY)) {
		r.sendToPlayer(playerID, protocol.EncodeTapRejected())
		return
	}

	cooldown := r.updatePlayerStats(player, now)
	r.broadcastTapResult(player, cooldown)
}

func (r *Room) validateTapRequest(player *domain.PlayerState, now int64) bool {
	if r.state.Phase != domain.PhasePlaying {
		return false
	}
	if !player.CanTap(now) {
		return false
	}
	return true
}

func (r *Room) decodeTapPayload(payload []byte) (float32, float32, bool) {
	if len(payload) < 8 {
		return 0, 0, false
	}
	tapX, tapY, _ := protocol.DecodeTap(payload)
	if math.IsNaN(float64(tapX)) || math.IsNaN(float64(tapY)) ||
		math.IsInf(float64(tapX), 0) || math.IsInf(float64(tapY), 0) ||
		float64(tapX) < 0 || float64(tapX) > 1 || float64(tapY) < 0 || float64(tapY) > 1 {
		return 0, 0, false
	}
	return tapX, tapY, true
}

func (r *Room) applyTapPhysics(tapX, tapY float64) bool {
	if !ApplyTapForce(&r.state.Balloon, tapX, tapY) {
		return false
	}
	ApplyGhostRepel(r.state, tapX, tapY)
	return true
}

func (r *Room) updatePlayerStats(player *domain.PlayerState, now int64) int64 {
	cooldown := CalculateCooldown(len(r.state.Players))
	player.RecordTap(now, cooldown)
	r.state.Balloon.Score++
	return cooldown
}

func (r *Room) broadcastTapResult(player *domain.PlayerState, cooldown int64) {
	tapMsg := protocol.EncodeTapAccepted(
		uint16(player.PlayerIndex), //nolint:gosec // G115: PlayerIndex < MaxPlayersPerRoom(50)
		uint32(cooldown),           //nolint:gosec // G115: cooldown bounded by CalculateCooldown
		float32(r.state.Balloon.X),
		float32(r.state.Balloon.Y),
	)
	r.broadcast(tapMsg, "")
}

func (r *Room) handleSetNicknameMsg(player *domain.PlayerState, payload []byte) {
	nickname, ok := protocol.DecodeNicknamePayload(payload)
	sanitized := ""
	if ok {
		sanitized = validate.Nickname(nickname)
	}
	if sanitized == "" {
		metrics.NicknameConfirmTotal.WithLabelValues("rejected").Inc()
		return
	}

	if sanitized == player.Nickname {
		player.NicknameConfirmed = true
		metrics.NicknameConfirmTotal.WithLabelValues("accepted").Inc()
		r.requestPersist()
		r.broadcast(r.buildSnapshot(), "")
		r.tryStartWhenAllReady()
		return
	}

	if !HandleSetNickname(r.state, player, sanitized, r.usedNames) {
		metrics.NicknameConfirmTotal.WithLabelValues("rejected").Inc()
		return
	}

	player.NicknameConfirmed = true
	metrics.NicknameConfirmTotal.WithLabelValues("accepted").Inc()

	r.requestPersist()
	r.broadcast(r.buildSnapshot(), "")
	r.tryStartWhenAllReady()
}

func (r *Room) handleRestartVoteMsg(player *domain.PlayerState, playerID string) {
	if err := HandleRestartVote(r, player); err != nil {
		r.logger.Warn("restart vote failed", "error", err, "player_id", playerID)
	}
}

func (r *Room) handlePingMsg(playerID string) {
	r.sendToPlayer(playerID, protocol.EncodePong())
}
