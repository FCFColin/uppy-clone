package handler

import (
	"log/slog"
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
