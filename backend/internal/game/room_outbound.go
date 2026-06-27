package game

import (
	"context"
	"log/slog"
	"time"

	"github.com/uppy-clone/backend/internal/metrics"
)

const outboundQueueSize = 256

type outboundMsg struct {
	payload         []byte
	excludePlayerID string
	critical        bool
	skipRedis       bool
}

type connTarget struct {
	playerID         string
	send             chan []byte
	consecutiveDrops *int
	pendingDisconnect *bool
	connClose        func()
}

// startOutboundLoop launches the per-room outbound delivery goroutine (once).
func (r *Room) startOutboundLoop() {
	r.outboundOnce.Do(func() {
		r.outboundCh = make(chan outboundMsg, outboundQueueSize)
		r.asyncWg.Add(1)
		go r.runOutboundLoop()
	})
}

func (r *Room) runOutboundLoop() {
	defer r.asyncWg.Done()
	for msg := range r.outboundCh {
		r.deliverOutbound(msg)
		metrics.SetRoomOutboundQueueDepth(r.state.LobbyCode, len(r.outboundCh))
	}
}

// enqueueOutbound queues a broadcast for async delivery. Caller must hold r.mu.
func (r *Room) enqueueOutbound(payload []byte, excludePlayerID string, critical, skipRedis bool) {
	copied := append([]byte(nil), payload...)
	msg := outboundMsg{
		payload:         copied,
		excludePlayerID: excludePlayerID,
		critical:        critical,
		skipRedis:       skipRedis,
	}
	if r.syncOutbound {
		targets := r.snapshotConnTargetsLocked(excludePlayerID)
		r.deliverToTargets(targets, msg)
		r.removePendingDisconnectsLocked()
		if !msg.skipRedis {
			r.publishBroadcastAsync(msg.payload, msg.excludePlayerID, msg.critical)
		}
		return
	}
	r.startOutboundLoop()
	select {
	case r.outboundCh <- msg:
		metrics.SetRoomOutboundQueueDepth(r.state.LobbyCode, len(r.outboundCh))
	default:
		if critical {
			r.outboundCh <- msg
			metrics.SetRoomOutboundQueueDepth(r.state.LobbyCode, len(r.outboundCh))
			return
		}
		metrics.WSMessagesDroppedTotal.WithLabelValues(r.state.LobbyCode).Inc()
		slog.Warn("outbound queue full, dropping non-critical message",
			"room_code", r.state.LobbyCode)
	}
}

func (r *Room) deliverOutbound(msg outboundMsg) {
	r.mu.Lock()
	targets := r.snapshotConnTargetsLocked(msg.excludePlayerID)
	r.mu.Unlock()
	r.deliverToTargets(targets, msg)
	r.mu.Lock()
	r.removePendingDisconnectsLocked()
	r.mu.Unlock()
	if !msg.skipRedis {
		r.publishBroadcastAsync(msg.payload, msg.excludePlayerID, msg.critical)
	}
}

func (r *Room) snapshotConnTargetsLocked(excludePlayerID string) []connTarget {
	targets := make([]connTarget, 0, len(r.connections))
	for pid, pc := range r.connections {
		if pid == excludePlayerID || pc == nil || pc.Send == nil {
			continue
		}
		pcCopy := pc
		targets = append(targets, connTarget{
			playerID:          pid,
			send:              pcCopy.Send,
			consecutiveDrops:  &pcCopy.consecutiveDrops,
			pendingDisconnect: &pcCopy.pendingDisconnect,
			connClose: func() {
				if pcCopy.Conn != nil {
					_ = pcCopy.Conn.Close()
				}
			},
		})
	}
	return targets
}

func (r *Room) deliverToTargets(targets []connTarget, msg outboundMsg) {
	for _, t := range targets {
		if msg.critical {
			select {
			case t.send <- msg.payload:
				*t.consecutiveDrops = 0
			case <-time.After(100 * time.Millisecond):
				slog.Error("critical message send timeout",
					"user_id", t.playerID,
					"room_code", r.state.LobbyCode)
			}
			continue
		}
		select {
		case t.send <- msg.payload:
			*t.consecutiveDrops = 0
		default:
			metrics.WSMessagesDroppedTotal.WithLabelValues(r.state.LobbyCode).Inc()
			*t.consecutiveDrops++
			drops := *t.consecutiveDrops
			if drops >= 3 {
				slog.Warn("slow client: messages being dropped",
					"user_id", t.playerID,
					"drops", drops,
					"room_code", r.state.LobbyCode)
			}
			if drops >= 10 {
				slog.Warn("disconnecting slow client",
					"user_id", t.playerID,
					"drops", drops,
					"room_code", r.state.LobbyCode)
				*t.pendingDisconnect = true
				t.connClose()
			}
		}
	}
}

func (r *Room) removePendingDisconnectsLocked() {
	for pid, pc := range r.connections {
		if pc != nil && pc.pendingDisconnect {
			delete(r.connections, pid)
			pc.pendingDisconnect = false
		}
	}
}

func (r *Room) publishBroadcastAsync(data []byte, excludePlayerID string, critical bool) {
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

func (r *Room) stopOutbound() {
	r.outboundOnce.Do(func() {}) // ensure channel exists
	if r.outboundCh != nil {
		close(r.outboundCh)
	}
}
