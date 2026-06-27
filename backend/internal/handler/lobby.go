package handler

import (
	"log/slog"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/game"
	"go.opentelemetry.io/otel/attribute"
)

// LobbyHandler handles lobby/room endpoints.
type LobbyHandler struct {
	hub            *game.Hub
	jwtMgr         *auth.JWTManager
	logger         *slog.Logger
	allowedOrigins []string
}

// NewLobbyHandler creates a new LobbyHandler.
func NewLobbyHandler(hub *game.Hub, jwtMgr *auth.JWTManager, allowedOrigins []string) *LobbyHandler {
	return &LobbyHandler{
		hub:            hub,
		jwtMgr:         jwtMgr,
		logger:         slog.Default().With("component", "lobby_handler"),
		allowedOrigins: allowedOrigins,
	}
}

// wsStaticSpanAttr is the pre-allocated static attribute shared by all WebSocket
// read/write pump spans.
var wsStaticSpanAttr = attribute.String("messaging.system", "websocket")
