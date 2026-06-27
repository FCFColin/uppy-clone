package handler

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (h *LobbyHandler) readPump(room *game.Room, playerID string, conn *websocket.Conn, wsCtx context.Context, cancel context.CancelFunc) {
	defer func() {
		cancel()
		_ = conn.Close()
		_ = room.HandleDisconnect(playerID)
		h.hub.DecrementWSConnection()
	}()
	conn.SetReadLimit(config.WSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(h.hub.Timeouts().WSPongTimeout))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(h.hub.Timeouts().WSPongTimeout))
		return nil
	})
	var tapSpanCounter uint64
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				h.logger.Warn("read error", "playerID", playerID, "error", err)
			}
			break
		}
		if len(message) == 0 {
			continue
		}
		msgType, payload := protocol.DecodeMessage(message)
		msgName := metrics.WSMessageTypeName(msgType)
		handleStart := time.Now()
		span := h.maybeStartReadSpan(wsCtx, room, playerID, msgType, &tapSpanCounter)
		if err := room.HandleMessage(playerID, msgType, payload); err != nil {
			if span != nil {
				span.RecordError(err)
			}
			h.logger.Error("handle message error", "playerID", playerID, "error", err)
		}
		metrics.RecordWSMessage(msgName, time.Since(handleStart))
		if span != nil {
			span.End()
		}
	}
}

func (h *LobbyHandler) maybeStartReadSpan(wsCtx context.Context, room *game.Room, playerID string, msgType byte, tapSpanCounter *uint64) trace.Span {
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
	case protocol.MsgPing:
		msgTypeName = "ping"
	default:
		msgTypeName = "unknown"
	}
	_, span := telemetry.Tracer().Start(wsCtx, "ws.readPump."+msgTypeName,
		trace.WithAttributes(wsStaticSpanAttr, attribute.String("messaging.destination", room.Code()), attribute.String("messaging.message_type", msgTypeName), attribute.String("messaging.player_id", playerID)),
	)
	return span
}
