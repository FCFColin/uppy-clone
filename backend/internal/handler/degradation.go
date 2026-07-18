package handler

import (
	"encoding/json"
	"net/http"

	"github.com/sony/gobreaker/v2"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
)

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
func RequireDB(db interface{}, w http.ResponseWriter) bool {
	if db != nil {
		return true
	}
	WriteDegradedJSON(w, http.StatusServiceUnavailable, nil, "Database temporarily unavailable")
	return false
}

// RequireRedis returns false and writes a degraded response when redis is nil.
func RequireRedis(redis interface{}, w http.ResponseWriter) bool {
	if redis != nil {
		return true
	}
	WriteDegradedJSON(w, http.StatusServiceUnavailable, nil, "Cache temporarily unavailable")
	return false
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
