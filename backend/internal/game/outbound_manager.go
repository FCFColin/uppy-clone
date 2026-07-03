package game

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
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
	playerID          string
	send              chan []byte
	consecutiveDrops  *int
	pendingDisconnect *bool
	connClose         func()
}

// ConnectionSource provides connection snapshots for outbound delivery.
type ConnectionSource interface {
	SnapshotTargets(excludePlayerID string) []connTarget
	RemovePendingDisconnects()
}

// OutboundManager handles per-room WebSocket message delivery.
type OutboundManager struct {
	ch     chan outboundMsg
	closed atomic.Bool
	once   sync.Once

	// syncOutbound is a pointer to Room.syncOutbound so tests can toggle it after construction.
	syncOutbound *bool

	broadcaster Broadcaster
	instanceID  string
	lobbyCode   string
	source      ConnectionSource
	logger      *slog.Logger
	asyncWG     *sync.WaitGroup
}

// NewOutboundManager creates an OutboundManager for a room.
func NewOutboundManager(lobbyCode, instanceID string, syncOutbound *bool, broadcaster Broadcaster, source ConnectionSource, logger *slog.Logger, asyncWG *sync.WaitGroup) *OutboundManager {
	return &OutboundManager{
		ch:           make(chan outboundMsg, outboundQueueSize),
		syncOutbound: syncOutbound,
		broadcaster:  broadcaster,
		instanceID:   instanceID,
		lobbyCode:    lobbyCode,
		source:       source,
		logger:       logger,
		asyncWG:      asyncWG,
	}
}

func (m *OutboundManager) startLoop() {
	m.once.Do(func() {
		m.asyncWG.Add(1)
		go m.runLoop()
	})
}

func (m *OutboundManager) runLoop() {
	defer m.asyncWG.Done()
	for msg := range m.ch {
		m.deliver(msg)
		metrics.SetRoomOutboundQueueDepth(m.lobbyCode, len(m.ch))
	}
}

func (m *OutboundManager) Enqueue(payload []byte, excludePlayerID string, critical, skipRedis bool) {
	if m.closed.Load() {
		return
	}
	copied := append([]byte(nil), payload...)
	msg := outboundMsg{
		payload:         copied,
		excludePlayerID: excludePlayerID,
		critical:        critical,
		skipRedis:       skipRedis,
	}
	if m.syncOutbound != nil && *m.syncOutbound {
		m.deliverLocked(excludePlayerID, msg)
		return
	}
	m.startLoop()
	func() {
		defer func() { recover() }()
		select {
		case m.ch <- msg:
			metrics.SetRoomOutboundQueueDepth(m.lobbyCode, len(m.ch))
		default:
			if critical {
				select {
				case m.ch <- msg:
					metrics.SetRoomOutboundQueueDepth(m.lobbyCode, len(m.ch))
				case <-time.After(100 * time.Millisecond):
					metrics.WSMessagesDroppedTotal.WithLabelValues(m.lobbyCode).Inc()
					slog.Warn("critical outbound queue blocked, dropping to avoid room lock hold",
						"room_code", m.lobbyCode)
				}
				return
			}
			metrics.WSMessagesDroppedTotal.WithLabelValues(m.lobbyCode).Inc()
			slog.Warn("outbound queue full, dropping non-critical message",
				"room_code", m.lobbyCode)
		}
	}()
}

func (m *OutboundManager) deliver(msg outboundMsg) {
	targets := m.source.SnapshotTargets(msg.excludePlayerID)
	m.deliverToTargets(targets, msg)
	m.source.RemovePendingDisconnects()
	m.publishIfNeeded(msg)
}

func (m *OutboundManager) deliverLocked(excludePlayerID string, msg outboundMsg) {
	targets := m.source.SnapshotTargets(excludePlayerID)
	m.deliverToTargets(targets, msg)
	m.source.RemovePendingDisconnects()
	m.publishIfNeeded(msg)
}

func (m *OutboundManager) publishIfNeeded(msg outboundMsg) {
	if !msg.skipRedis {
		m.publishBroadcastAsync(msg.payload, msg.excludePlayerID, msg.critical)
	}
}

func (m *OutboundManager) deliverToTargets(targets []connTarget, msg outboundMsg) {
	for _, t := range targets {
		func() {
			defer func() { recover() }()
			if msg.critical {
				m.deliverCritical(t, msg)
				return
			}
			m.deliverNonCritical(t, msg)
		}()
	}
}

func (m *OutboundManager) deliverCritical(t connTarget, msg outboundMsg) {
	select {
	case t.send <- msg.payload:
		*t.consecutiveDrops = 0
	case <-time.After(100 * time.Millisecond):
		slog.Error("critical message send timeout",
			"user_id", t.playerID,
			"room_code", m.lobbyCode)
	}
}

func (m *OutboundManager) deliverNonCritical(t connTarget, msg outboundMsg) {
	select {
	case t.send <- msg.payload:
		*t.consecutiveDrops = 0
	default:
		metrics.WSMessagesDroppedTotal.WithLabelValues(m.lobbyCode).Inc()
		*t.consecutiveDrops++
		m.checkSlowClient(t)
	}
}

func (m *OutboundManager) checkSlowClient(t connTarget) {
	drops := *t.consecutiveDrops
	if drops >= 10 {
		slog.Warn("disconnecting slow client",
			"user_id", t.playerID,
			"drops", drops,
			"room_code", m.lobbyCode)
		*t.pendingDisconnect = true
		t.connClose()
	} else if drops >= 3 {
		slog.Warn("slow client: messages being dropped",
			"user_id", t.playerID,
			"drops", drops,
			"room_code", m.lobbyCode)
	}
}

func (m *OutboundManager) publishBroadcastAsync(data []byte, excludePlayerID string, critical bool) {
	if m.broadcaster == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	msg := BroadcastMessage{
		RoomCode:        m.lobbyCode,
		ExcludePlayer:   excludePlayerID,
		ExcludeInstance: m.instanceID,
		Payload:         data,
		Critical:        critical,
	}
	if err := m.broadcaster.Publish(ctx, m.lobbyCode, msg); err != nil {
		m.logger.Warn("redis publish failed, local-only delivery",
			"error", err,
			"room", m.lobbyCode)
	}
}

func (m *OutboundManager) Stop() {
	m.closed.Store(true)
	if m.ch != nil {
		close(m.ch)
	}
}

func (m *OutboundManager) OutboundCh() chan outboundMsg { return m.ch }
