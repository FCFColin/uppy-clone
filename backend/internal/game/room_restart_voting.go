package game

import (
	"fmt"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── Restart Voting ──────────────────────────────────────────────────

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
		return CheckRestartConsensus(room)
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
	yesVotes, connectedCount := countRestartYesVotes(room.state.Players, room.state.RestartVotes)

	// 广播投票状态
	var countdownMs uint32
	if room.state.RestartTimerStart != nil {
		elapsed := time.Now().UnixMilli() - *room.state.RestartTimerStart
		remaining := int64(domain.RestartTimeoutMs) - elapsed
		if remaining < 0 {
			remaining = 0
		}
		countdownMs = uint32(remaining) //nolint:gosec // G115: bounded by CountdownTicks
	}

	room.broadcast(protocol.EncodeRestartStatus(
		uint8(yesVotes),       //nolint:gosec // G115: yesVotes <= connectedCount <= MaxPlayersPerRoom(50) < 256
		uint8(connectedCount), //nolint:gosec // G115: connectedCount <= MaxPlayersPerRoom(50) < 256
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
		room.setEndGameAlarm(time.Now().Add(time.Duration(domain.RestartTimeoutMs) * time.Millisecond))
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
	room.connMu.RLock()
	for pid := range room.connections {
		activePlayerIDs[pid] = true
	}
	room.connMu.RUnlock()

	cleanupDisconnectedPlayers(room, players, activePlayerIDs)

	// 清理后无活跃玩家时，重置为 waiting
	if len(activePlayerIDs) == 0 {
		room.state = NewGameState(string(room.state.LobbyCode), room.state.RNGSeed, room.rng)
		room.state.Phase = domain.PhaseWaiting
		room.stopTick()
		// game-021: Broadcast state change even with no active players —
		// a reconnecting player may receive the updated phase.
		room.broadcast(protocol.EncodeGameStateChange(protocol.PhaseWaiting), "")
		room.broadcast(room.buildSnapshot(), "")
		room.requestPersist()
		return nil
	}

	room.state = buildRestartState(string(room.state.LobbyCode), players, nextPlayerIndex, room.state.RNGSeed, room.rng)
	// game-022: Removed redundant ResetGameEntities call — buildRestartState
	// already initializes balloon, bird, ghost, and wind via NewGameState.
	// Calling ResetGameEntities again consumed RNG a second time, causing
	// deterministic divergence between server restart iterations.
	room.countdownStart = time.Now().UnixMilli()

	room.scheduleCountdownFromNow()

	room.requestPersist()

	room.broadcastCountdownPhase()

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
func buildRestartState(lobbyCode string, players map[string]*domain.PlayerState, nextPlayerIndex int, seed int64, rng RNGSource) *domain.GameState {
	state := NewGameState(lobbyCode, seed, rng)
	state.RestartVotes = make(map[string]bool)
	state.RestartTimerStart = nil
	state.Players = players
	state.NextPlayerIndex = nextPlayerIndex

	// 直接开始新局（通过倒计时）
	state.Phase = domain.PhaseCountdown
	state.StartedAt = time.Now().UnixMilli()
	state.SessionID = domain.UUID()

	return state
}

func countRestartYesVotes(players map[string]*domain.PlayerState, votes map[string]bool) (yes, connected int) {
	for _, p := range players {
		if !p.Disconnected {
			connected++
			if v, ok := votes[p.ID]; ok && v {
				yes++
			}
		}
	}
	return yes, connected
}
