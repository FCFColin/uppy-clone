package handler

import (
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
)

// LobbyHandler handles lobby/room endpoints.
type LobbyHandler struct {
	hub            GameService
	logger         *slog.Logger
	allowedOrigins []string
}

// NewLobbyHandler creates a new LobbyHandler.
func NewLobbyHandler(hub GameService, allowedOrigins []string) *LobbyHandler {
	return &LobbyHandler{
		hub:            hub,
		logger:         slog.Default().With("component", "lobby_handler"),
		allowedOrigins: allowedOrigins,
	}
}

// wsStaticSpanAttr is the pre-allocated static attribute shared by all WebSocket
// read/write pump spans.
var wsStaticSpanAttr = attribute.String("messaging.system", "websocket")
