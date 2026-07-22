package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/go-chi/chi/v5"
	"github.com/sony/gobreaker/v2"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
)

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

// ─── Degradation Infrastructure ──────────────────────────────────────

// DegradedResponse 非关键依赖不可用时的部分可用响应（见 ADR-004）。
type DegradedResponse struct {
	Data     interface{} `json:"data"`
	Degraded bool        `json:"degraded"`
	Message  string      `json:"message,omitempty"`
}

// WriteDegradedJSON writes a degraded response with the given status, data, and message.
func WriteDegradedJSON(w http.ResponseWriter, status int, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(DegradedResponse{
		Data:     data,
		Degraded: true,
		Message:  message,
	})
}

// RequireDB returns false and writes a degraded response when db is nil.
// Handles Go's typed-nil trap: a (*T)(nil) boxed into interface{} is non-nil
// at the interface level but represents a nil pointer that must be guarded.
func RequireDB(db interface{}, w http.ResponseWriter) bool {
	if !isNilPointer(db) {
		return true
	}
	WriteDegradedJSON(w, http.StatusServiceUnavailable, nil, "Database temporarily unavailable")
	return false
}

// RequireRedis returns false and writes a degraded response when redis is nil.
func RequireRedis(redis interface{}, w http.ResponseWriter) bool {
	if !isNilPointer(redis) {
		return true
	}
	WriteDegradedJSON(w, http.StatusServiceUnavailable, nil, "Cache temporarily unavailable")
	return false
}

// isNilPointer reports whether v is an untyped nil interface OR a nil
// pointer behind a non-nil interface (Go's typed-nil trap).
func isNilPointer(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// RequireHub returns false and writes 503 when hub is nil.
func RequireHub(hub interface{ RoomCount() int }, w http.ResponseWriter) bool {
	if hub != nil {
		return true
	}
	domain.ServiceUnavailable("Room service temporarily unavailable").Write(w)
	return false
}

// RequireHubDegraded returns false and writes a degraded JSON response when hub is nil.
func RequireHubDegraded(hub *game.Hub, w http.ResponseWriter, status int, payload interface{}, message string) bool {
	if hub != nil {
		return true
	}
	WriteDegradedJSON(w, status, payload, message)
	return false
}

// IsDegraded returns true if any circuit breaker is in an open or half-open state.
func IsDegraded(cbs ...*gobreaker.CircuitBreaker[any]) bool {
	for _, cb := range cbs {
		if cb != nil {
			state := cb.State()
			if state == gobreaker.StateOpen || state == gobreaker.StateHalfOpen {
				return true
			}
		}
	}
	return false
}

// DegradedHandler returns an HTTP handler that reports overall degradation status
// based on the provided circuit breakers. Returns { "degraded": true/false }.
func DegradedHandler(cbs ...*gobreaker.CircuitBreaker[any]) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{degradedKey: IsDegraded(cbs...)})
	}
}

// writeJSON sets the JSON content type, writes the status code, and encodes the
// payload as JSON. It is the standard handler response helper.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
