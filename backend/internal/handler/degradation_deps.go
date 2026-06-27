package handler

import (
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/store"
)

// RequireDB returns false and writes a degraded response when db is nil.
func RequireDB(db *store.PostgresStore, w http.ResponseWriter) bool {
	if db != nil {
		return true
	}
	WriteDegradedJSON(w, http.StatusServiceUnavailable, nil, "Database temporarily unavailable")
	return false
}

// RequireRedis returns false and writes a degraded response when redis is nil.
func RequireRedis(redis *store.RedisStore, w http.ResponseWriter) bool {
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
	apierror.New(http.StatusServiceUnavailable, "Service Unavailable", "Room service temporarily unavailable").Write(w)
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
