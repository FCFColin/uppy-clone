package handler

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (h *LobbyHandler) startWSPumps(room *game.Room, userId string, conn *websocket.Conn, reqCtx context.Context) {
	if err := room.HandleJoin(userId, conn); err != nil {
		h.logger.Error("handle join failed", "error", err)
		h.hub.DecrementWSConnection()
		_ = conn.Close()
		return
	}

	wsCtx, cancel := context.WithCancel(reqCtx)
	go h.writePump(room, userId, conn, wsCtx)
	h.readPump(room, userId, conn, wsCtx, cancel)
}

func (h *LobbyHandler) writePump(room *game.Room, playerID string, conn *websocket.Conn, wsCtx context.Context) {
	pc := room.GetConnection(playerID)
	if pc == nil {
		return
	}

	ticker := time.NewTicker(h.hub.Timeouts().WSPingInterval)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	for {
		select {
		case <-wsCtx.Done():
			return
		case msg, ok := <-pc.Send:
			_ = conn.SetWriteDeadline(time.Now().Add(h.hub.Timeouts().WSWriteTimeout))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			_, span := telemetry.Tracer().Start(wsCtx, "ws.writePump.broadcast",
				trace.WithAttributes(
					wsStaticSpanAttr,
					attribute.String("messaging.destination", room.Code()),
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
			_ = conn.SetWriteDeadline(time.Now().Add(h.hub.Timeouts().WSWriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
