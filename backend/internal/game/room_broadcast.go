package game

import (
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// broadcast 发送数据给所有连接（可排除一个玩家），并发布到 Redis 供跨实例投递。
//
// P4-5: 发送缓冲区满时记录 ws_messages_dropped_total 指标，并跟踪连续丢弃次数。
// 连续 3 次丢弃记录 WARN 日志，连续 10 次丢弃强制断开慢客户端。
// 调用方必须持有 r.mu 锁。实际投递在 outbound goroutine 中完成（不持锁）。
func (r *Room) broadcast(data []byte, excludePlayerID string) {
	r.enqueueOutbound(data, excludePlayerID, false, false)
}

// broadcastLocal 仅向本地连接投递数据，不发布到 Redis。
// 由 Hub 在收到 Redis Pub/Sub 远程消息时调用，避免回环。
func (r *Room) broadcastLocal(data []byte, excludePlayerID string) {
	r.enqueueOutbound(data, excludePlayerID, false, true)
}

// broadcastCritical sends a critical phase message with blocking delivery per client.
// 调用方必须持有 r.mu 锁。
func (r *Room) broadcastCritical(message []byte) {
	r.enqueueOutbound(message, "", true, false)
}

// sendToPlayer 发送数据给指定玩家（同步非阻塞，单玩家路径足够快）。
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
	players := r.players[:0]
	for _, p := range r.state.Players {
		if p.Disconnected {
			continue
		}
		cooldownRemaining := int64(0)
		now := time.Now().UnixMilli()
		if p.CooldownEndTime > now {
			cooldownRemaining = p.CooldownEndTime - now
		}
		players = append(players, protocol.PlayerState{
			PlayerIndex:       uint16(p.PlayerIndex),       //nolint:gosec:G115 // bounded
			CooldownMs:        uint32(cooldownRemaining),   //nolint:gosec:G115 // bounded by cooldown duration
			Palette:           uint32(p.Palette),           //nolint:gosec:G115 // Palette < 8
			ScoreContribution: uint32(p.ScoreContribution), //nolint:gosec:G115 // game score, non-negative
			Nickname:          p.Nickname,
		})
	}
	r.players = players

	return protocol.EncodeSnapshot(
		modelPhaseToProtocol(r.state.Phase),
		uint32(r.state.TickCount), //nolint:gosec:G115 // tick counter wraps naturally
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
			RepelTimer: uint16(r.state.Ghost.RepelTimer), //nolint:gosec:G115 // bounded timer
		},
		players,
		nil,
		r.state.Wind,
	)
}
