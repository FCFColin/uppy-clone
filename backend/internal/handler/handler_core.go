package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/uppy-clone/backend/internal/game"
)

// ─── Response Helper ────────────────────────────────────────────────

// writeJSON sets the JSON content type, writes the status code, and encodes the
// payload as JSON. It is the standard handler response helper.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ─── URL Parameter Helper ───────────────────────────────────────────

// URLParam extracts a named path parameter from the request.
//
// It prefers the Go 1.22+ standard library path value (r.PathValue) so that
// tests can set parameters via r.SetPathValue without a router, and falls back
// to chi.URLParam for production routes served by the chi router. This keeps
// the handler package's tests decoupled from chi-specific context plumbing.
func URLParam(r *http.Request, name string) string {
	if v := r.PathValue(name); v != "" {
		return v
	}
	return chi.URLParam(r, name)
}

// ─── Config ─────────────────────────────────────────────────────────

// Config holds application configuration passed to handlers.
type Config struct {
	ResendAPIKey       string
	EmailFrom          string
	AdminPassword      string
	JWTPrivateKey      string
	JWTPublicKey       string
	AdminJWTPrivateKey string
	AdminJWTPublicKey  string
	DatabaseURL        string
	RedisURL           string
	RedisEphemeralURL  string
	RedisPubSubURL     string
	Port               string
	FrontendDir        string

	// EnableEmbeddedWorkers controls in-process worker startup. When false,
	// startWorkers skips GameResult/Outbox/GDPR workers (standalone deployment).
	EnableEmbeddedWorkers bool
}

// ─── LobbyHandler ───────────────────────────────────────────────────

// LobbyHandler handles lobby/room endpoints.
type LobbyHandler struct {
	hub            *game.Hub
	logger         *slog.Logger
	allowedOrigins []string
}

// NewLobbyHandler creates a new LobbyHandler.
func NewLobbyHandler(hub *game.Hub, allowedOrigins []string) *LobbyHandler {
	return &LobbyHandler{
		hub:            hub,
		logger:         slog.Default().With("component", "lobby_handler"),
		allowedOrigins: allowedOrigins,
	}
}
