package game

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ─── OutboundManager ─────────────────────────────────────────────────

const outboundQueueSize = 256

type outboundMsg struct {
	payload         []byte
	excludePlayerID string
	critical        bool
}

type connTarget struct {
	playerID          string
	send              chan []byte
	consecutiveDrops  *atomic.Int64
	pendingDisconnect *atomic.Bool
	connClose         func()
}

// OutboundSource is the narrow interface OutboundManager needs from its owning
// Room. Declared with public methods so it can be satisfied across package
// boundaries — this replaces the previous source *Room field which called
// private methods (e.g. observer()) and therefore could not actually be
// extracted to a separate package despite the comment claiming otherwise.
//
// *Room satisfies this interface via:
//   - SnapshotTargets        (promoted from embedded RoomConnections)
//   - RemovePendingDisconnects
//   - Observer               (public wrapper around the former observer())
type OutboundSource interface {
	SnapshotTargets(excludePlayerID string) []connTarget
	RemovePendingDisconnects()
	Observer() GameObserver
}

// OutboundManager handles per-room WebSocket message delivery.
type OutboundManager struct {
	ch        chan outboundMsg
	closed    atomic.Bool
	once      sync.Once
	closeOnce sync.Once

	// syncOutbound is a pointer to Room.syncOutbound so tests can toggle it after construction.
	syncOutbound *bool

	lobbyCode string
	source    OutboundSource
	logger    *slog.Logger
	asyncWG   *sync.WaitGroup
}

// NewOutboundManager creates an OutboundManager for a room.
// source must implement OutboundSource (*Room does).
func NewOutboundManager(lobbyCode string, syncOutbound *bool, source OutboundSource, logger *slog.Logger, asyncWG *sync.WaitGroup) *OutboundManager {
	return &OutboundManager{
		ch:           make(chan outboundMsg, outboundQueueSize),
		syncOutbound: syncOutbound,
		lobbyCode:    lobbyCode,
		source:       source,
		logger:       logger,
		asyncWG:      asyncWG,
	}
}

// observer returns the GameObserver through the source Room → Hub chain.
func (m *OutboundManager) observer() GameObserver {
	if m.source != nil {
		return m.source.Observer()
	}
	return NoopGameObserver{}
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
		m.observer().SetOutboundQueueDepth(m.lobbyCode, len(m.ch))
	}
}

// Enqueue queues a message for asynchronous delivery, optionally excluding a player and marking it critical.
func (m *OutboundManager) Enqueue(payload []byte, excludePlayerID string, critical bool) {
	if m.closed.Load() {
		return
	}
	copied := append([]byte(nil), payload...)
	msg := outboundMsg{
		payload:         copied,
		excludePlayerID: excludePlayerID,
		critical:        critical,
	}
	if m.syncOutbound != nil && *m.syncOutbound {
		m.deliverLocked(excludePlayerID, msg)
		return
	}
	m.startLoop()
	func() {
		defer m.recoverPanic("enqueue")
		select {
		case m.ch <- msg:
			m.observer().SetOutboundQueueDepth(m.lobbyCode, len(m.ch))
		default:
			if critical {
				m.enqueueCritical(msg)
				return
			}
			m.droppedNonCritical()
		}
	}()
}

func (m *OutboundManager) enqueueCritical(msg outboundMsg) {
	select {
	case m.ch <- msg:
		m.observer().SetOutboundQueueDepth(m.lobbyCode, len(m.ch))
	case <-time.After(2 * time.Second):
		m.observer().IncWSMessageDropped(m.lobbyCode)
		slog.Error("critical outbound queue blocked for 2s, dropping",
			"room_code", m.lobbyCode)
	}
}

func (m *OutboundManager) droppedNonCritical() {
	m.observer().IncWSMessageDropped(m.lobbyCode)
	slog.Warn("outbound queue full, dropping non-critical message",
		"room_code", m.lobbyCode)
}

func (m *OutboundManager) deliver(msg outboundMsg) {
	m.deliverLocked(msg.excludePlayerID, msg)
}

func (m *OutboundManager) deliverLocked(excludePlayerID string, msg outboundMsg) {
	targets := m.source.SnapshotTargets(excludePlayerID)
	m.deliverToTargets(targets, msg)
	m.source.RemovePendingDisconnects()
}

func (m *OutboundManager) deliverToTargets(targets []connTarget, msg outboundMsg) {
	for _, t := range targets {
		func() {
			defer m.recoverPanic("deliverToTargets")
			if msg.critical {
				m.deliverCritical(t, msg)
				return
			}
			m.deliverNonCritical(t, msg)
		}()
	}
}

func (m *OutboundManager) recoverPanic(context string) {
	if r := recover(); r != nil {
		slog.Warn("panic recovered in outbound "+context, "panic", r, "room_code", m.lobbyCode)
	}
}

func (m *OutboundManager) deliverCritical(t connTarget, msg outboundMsg) {
	select {
	case t.send <- msg.payload:
		t.consecutiveDrops.Store(0)
	case <-time.After(100 * time.Millisecond):
		slog.Error("critical message send timeout",
			"user_id", t.playerID,
			"room_code", m.lobbyCode)
	}
}

func (m *OutboundManager) deliverNonCritical(t connTarget, msg outboundMsg) {
	select {
	case t.send <- msg.payload:
		t.consecutiveDrops.Store(0)
	default:
		m.observer().IncWSMessageDropped(m.lobbyCode)
		t.consecutiveDrops.Add(1)
		m.checkSlowClient(t)
	}
}

func (m *OutboundManager) checkSlowClient(t connTarget) {
	drops := t.consecutiveDrops.Load()
	if drops >= 10 {
		slog.Warn("disconnecting slow client",
			"user_id", t.playerID,
			"drops", drops,
			"room_code", m.lobbyCode)
		t.pendingDisconnect.Store(true)
		t.connClose()
	} else if drops >= 3 {
		slog.Warn("slow client: messages being dropped",
			"user_id", t.playerID,
			"drops", drops,
			"room_code", m.lobbyCode)
	}
}

// Stop closes the outbound channel and prevents further message enqueuing.
func (m *OutboundManager) Stop() {
	m.closed.Store(true)
	m.closeOnce.Do(func() {
		if m.ch != nil {
			close(m.ch)
		}
	})
}

// OutboundCh returns the outbound message channel.
func (m *OutboundManager) OutboundCh() chan outboundMsg { return m.ch }
