package game

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

// OutboundManager handles per-room WebSocket message delivery.
type OutboundManager struct {
	ch        chan outboundMsg
	closed    atomic.Bool
	once      sync.Once
	closeOnce sync.Once

	// syncOutbound is a pointer to Room.syncOutbound so tests can toggle it after construction.
	syncOutbound *bool

	lobbyCode string
	source    *Room
	logger    *slog.Logger
	asyncWG   *sync.WaitGroup
}

// NewOutboundManager creates an OutboundManager for a room.
func NewOutboundManager(lobbyCode string, syncOutbound *bool, source *Room, logger *slog.Logger, asyncWG *sync.WaitGroup) *OutboundManager {
	return &OutboundManager{
		ch:           make(chan outboundMsg, outboundQueueSize),
		syncOutbound: syncOutbound,
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
			metrics.SetRoomOutboundQueueDepth(m.lobbyCode, len(m.ch))
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
		metrics.SetRoomOutboundQueueDepth(m.lobbyCode, len(m.ch))
	case <-time.After(2 * time.Second):
		metrics.WSMessagesDroppedTotal.WithLabelValues(m.lobbyCode).Inc()
		slog.Error("critical outbound queue blocked for 2s, dropping",
			"room_code", m.lobbyCode)
	}
}

func (m *OutboundManager) droppedNonCritical() {
	metrics.WSMessagesDroppedTotal.WithLabelValues(m.lobbyCode).Inc()
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
		metrics.WSMessagesDroppedTotal.WithLabelValues(m.lobbyCode).Inc()
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

// ─── RoomHandle Interface ────────────────────────────────────────────

// RoomHandle is the narrow interface exposed to packages that only need to run
// a WebSocket session on a room (e.g. the handler package). Returning this
// interface from Hub.GetRoom decouples callers from the concrete *Room type and
// its internal fields.
type RoomHandle interface {
	RunSession(reqCtx context.Context, playerID string, conn *websocket.Conn) error
}

// ─── WSSession (extracted from Room to reduce God-object surface) ────

// wsStaticSpanAttr is the pre-allocated static attribute shared by all WebSocket
// read/write pump spans.
var wsStaticSpanAttr = attribute.String("messaging.system", "websocket")

// WSSession encapsulates the WebSocket read/write pump logic for a single player
// session. Extracted from Room to keep Room focused on game state management.
type WSSession struct {
	room *Room
}

// RunSession drives a single player's WebSocket session: it joins the player to
// the room, then runs the read/write pumps until the connection closes. It
// blocks until the session ends. The caller is responsible for reserving and
// releasing the WebSocket connection slot (TryReserveWSConnection /
// DecrementWSConnection) on the Hub.
func (s *WSSession) RunSession(reqCtx context.Context, playerID string, conn *websocket.Conn) error {
	r := s.room
	if err := r.HandleJoin(playerID, conn); err != nil {
		r.logger.Error("handle join failed", "error", err)
		_ = conn.Close()
		return err
	}

	wsCtx, cancel := context.WithTimeout(reqCtx, r.timeouts.WSHandlerTimeout)
	go s.writePump(playerID, conn, wsCtx)
	s.readPump(playerID, conn, wsCtx, cancel)
	return nil
}

func (s *WSSession) writePump(playerID string, conn *websocket.Conn, wsCtx context.Context) {
	r := s.room
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("writePump panic recovered", "playerID", playerID, "room", r.Code(), "panic", rec)
			_ = conn.Close()
		}
	}()

	pc := r.GetConnection(playerID)
	if pc == nil {
		return
	}

	ticker := time.NewTicker(r.timeouts.WSPingInterval)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	for {
		select {
		case <-wsCtx.Done():
			return
		case msg, ok := <-pc.Send:
			_ = conn.SetWriteDeadline(time.Now().Add(r.timeouts.WSWriteTimeout))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			_, span := tracer.Start(wsCtx, "ws.writePump.broadcast",
				trace.WithAttributes(
					wsStaticSpanAttr,
					attribute.String("messaging.destination", r.Code()),
					attribute.String("messaging.player_id", playerID),
					attribute.Int("messaging.message_size", len(msg)),
				),
			)
			if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				span.RecordError(err)
				span.End()
				return
			}
			span.End()

		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(r.timeouts.WSWriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *WSSession) readPump(playerID string, conn *websocket.Conn, wsCtx context.Context, cancel context.CancelFunc) {
	r := s.room
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("readPump panic recovered", "playerID", playerID, "room", r.Code(), "panic", rec)
		}
	}()
	defer func() {
		cancel()
		_ = conn.Close()
		_ = r.HandleDisconnect(playerID)
	}()
	conn.SetReadLimit(config.WSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(r.timeouts.WSPongTimeout))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(r.timeouts.WSPongTimeout))
		return nil
	})
	var tapSpanCounter uint64
	for {
		_ = conn.SetReadDeadline(time.Now().Add(r.timeouts.WSPongTimeout))
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				r.logger.Warn("read error", "playerID", playerID, "error", err)
			}
			break
		}
		if len(message) == 0 {
			continue
		}
		msgType, payload := protocol.DecodeMessage(message)
		msgName := protocol.WSMessageTypeName(msgType)
		handleStart := time.Now()
		span := s.maybeStartReadSpan(wsCtx, playerID, msgType, &tapSpanCounter)
		if err := r.HandleMessage(playerID, msgType, payload); err != nil {
			if span != nil {
				span.RecordError(err)
			}
			r.logger.Error("handle message error", "playerID", playerID, "error", err)
		}
		metrics.RecordWSMessage(msgName, time.Since(handleStart))
		if span != nil {
			span.End()
		}
	}
}

func (s *WSSession) maybeStartReadSpan(wsCtx context.Context, playerID string, msgType byte, tapSpanCounter *uint64) trace.Span {
	r := s.room
	createSpan := true
	switch msgType {
	case protocol.MsgPing:
		createSpan = false
	case protocol.MsgTap:
		*tapSpanCounter++
		if *tapSpanCounter%100 != 0 {
			createSpan = false
		}
	}
	if !createSpan {
		return nil
	}
	var msgTypeName string
	switch msgType {
	case protocol.MsgTap:
		msgTypeName = "tap"
	case protocol.MsgSetNickname:
		msgTypeName = "set_nickname"
	case protocol.MsgRestartVote:
		msgTypeName = "restart_vote"
	default:
		msgTypeName = unknownPlayerID
	}
	_, span := tracer.Start(wsCtx, "ws.readPump."+msgTypeName,
		trace.WithAttributes(wsStaticSpanAttr, attribute.String("messaging.destination", r.Code()), attribute.String("messaging.message_type", msgTypeName), attribute.String("messaging.player_id", playerID)),
	)
	return span
}
