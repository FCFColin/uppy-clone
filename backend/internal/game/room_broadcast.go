package game

import (
	"context"
	"log/slog"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
)

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
		if pc == nil || pc.Send == nil {
			continue
		}
		select {
		case pc.Send <- data:
			pc.consecutiveDrops = 0
		default:
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
				if pc.Conn != nil {
					_ = pc.Conn.Close()
				}
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
		nil,
		r.state.Wind,
	)
}
