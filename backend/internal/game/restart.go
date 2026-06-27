package game

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
	"github.com/uppy-clone/backend/internal/protocol"
)

// HandleRestartVote 处理重启投票请求
//
// 仅在 ended 阶段有效，记录玩家投票并检查共识。
// 调用方必须持有 room.mu 锁。
func HandleRestartVote(room *Room, player *domain.PlayerState) error {
	if room.state.Phase != domain.PhaseEnded {
		return nil
	}

	// 重复投票：若仍在 ended 且已达共识，允许重试（上次 RestartAndStart 可能失败）
	if _, ok := room.state.RestartVotes[player.ID]; ok {
		if room.state.Phase == domain.PhaseEnded {
			return CheckRestartConsensus(room)
		}
		return nil
	}

	room.state.RestartVotes[player.ID] = true
	return CheckRestartConsensus(room)
}

// CheckRestartConsensus 检查重启投票共识
//
// - 广播当前投票状态（让所有客户端看到投票进度）
// - 全部同意 → 立即重启
// - 第一个投票 → 启动 30 秒倒计时
func CheckRestartConsensus(room *Room) error {
	// 计算在线玩家和投票数
	connectedCount := 0
	yesVotes := 0
	for _, p := range room.state.Players {
		if !p.Disconnected {
			connectedCount++
			if v, ok := room.state.RestartVotes[p.ID]; ok && v {
				yesVotes++
			}
		}
	}

	// 广播投票状态
	var countdownMs uint32
	if room.state.RestartTimerStart != nil {
		elapsed := time.Now().UnixMilli() - *room.state.RestartTimerStart
		remaining := int64(protocol.RestartTimeoutMs) - elapsed
		if remaining < 0 {
			remaining = 0
		}
		countdownMs = uint32(remaining) //nolint:gosec // bounded by CountdownTicks
	}

	room.broadcast(protocol.EncodeRestartStatus(
		uint8(yesVotes),       //nolint:gosec // yesVotes <= connectedCount <= MaxPlayersPerRoom(50) < 256
		uint8(connectedCount), //nolint:gosec // connectedCount <= MaxPlayersPerRoom(50) < 256
		countdownMs,
	), "")

	if room.state.Phase != domain.PhaseEnded {
		return nil
	}

	// 全部同意 → 立即重启
	if yesVotes >= connectedCount && connectedCount > 0 {
		return RestartAndStart(room)
	}

	// 第一个投票 → 启动 30 秒倒计时
	if room.state.RestartTimerStart == nil {
		now := time.Now().UnixMilli()
		room.state.RestartTimerStart = &now
		// 设置定时器（通过 tick 循环或外部调度器处理）
		// 在 Room 中，我们通过设置 endGameAlarm 来处理
		room.setEndGameAlarm(time.Now().Add(time.Duration(protocol.RestartTimeoutMs) * time.Millisecond))
	}

	return nil
}

// RestartAndStart 不关闭 WebSocket 的重启：重置状态 + 立即开始新局
func RestartAndStart(room *Room) error {
	if room.state.Phase != domain.PhaseEnded {
		return fmt.Errorf("restartAndStart: phase is %s, not ended", room.state.Phase)
	}

	// 保留玩家信息和索引分配器
	players := make(map[string]*domain.PlayerState)
	for k, v := range room.state.Players {
		players[k] = v
	}
	nextPlayerIndex := room.state.NextPlayerIndex

	activePlayerIDs := make(map[string]bool)
	for pid := range room.connections {
		activePlayerIDs[pid] = true
	}

	cleanupDisconnectedPlayers(room, players, activePlayerIDs)

	// 清理后无活跃玩家时，重置为 waiting
	if len(activePlayerIDs) == 0 {
		room.state = NewGameState(room.state.LobbyCode)
		room.state.Phase = domain.PhaseWaiting
		room.stopTick()
		room.saveState()
		return nil
	}

	// P4-6.1: Saga 补偿模式 — 先持久化再广播，失败时回滚内存状态。
	// 捕获旧状态用于回滚
	oldState := room.state

	room.state = buildRestartState(room.state.LobbyCode, players, nextPlayerIndex)
	ResetGameEntities(room.state, RandomSpawnTimer())
	room.countdownStart = time.Now().UnixMilli()

	countdownMs := int64(protocol.CountdownTicks) * 1000 / int64(protocol.TickRate)
	room.setEndGameAlarm(time.Now().Add(time.Duration(countdownMs) * time.Millisecond))

	// 先持久化，成功后再广播。持久化失败时回滚内存状态，不广播不一致的状态。
	if err := room.saveStateWithError(); err != nil {
		slog.Error("restart: failed to save state, aborting", "error", err)
		room.state = oldState
		return err
	}

	countdownMsU32 := uint32(protocol.CountdownTicks) * 1000 / uint32(protocol.TickRate)
	room.broadcastCritical(protocol.EncodeGameStateChange(protocol.PhaseCountdown, countdownMsU32))
	room.broadcast(room.buildSnapshot(), "")

	return nil
}

// cleanupDisconnectedPlayers removes disconnected players from the map and resets
// stats for remaining players.
func cleanupDisconnectedPlayers(room *Room, players map[string]*domain.PlayerState, activePlayerIDs map[string]bool) {
	for pid, player := range players {
		if !activePlayerIDs[pid] || player.Disconnected {
			delete(players, pid)
			delete(room.usedNames, player.Nickname)
		} else {
			// 重置统计
			player.ScoreContribution = 0
			player.TapsCount = 0
			player.CooldownEndTime = time.Now().UnixMilli()
			player.MessageCount = 0
			player.MessageWindowStart = 0
			player.LastNicknameChange = 0
		}
	}
}

// buildRestartState creates a fresh GameState for a restart, preserving players
// and the next-player index, then transitioning to the countdown phase.
func buildRestartState(lobbyCode string, players map[string]*domain.PlayerState, nextPlayerIndex int) *domain.GameState {
	state := NewGameState(lobbyCode)
	state.RestartVotes = make(map[string]bool)
	state.RestartTimerStart = nil
	state.Players = players
	state.NextPlayerIndex = nextPlayerIndex

	// 直接开始新局（通过倒计时）
	state.Phase = domain.PhaseCountdown
	state.StartedAt = time.Now().UnixMilli()
	state.SessionID = idgen.UUID()

	return state
}
