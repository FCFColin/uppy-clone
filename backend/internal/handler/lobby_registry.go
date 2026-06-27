package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/metrics"
)

// CreateRoom handles POST /api/registry/create
func (h *LobbyHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if !RequireHubDegraded(h.hub, w, http.StatusServiceUnavailable,
		map[string]string{"code": ""},
		"Room service temporarily unavailable, please retry") {
		slog.Warn("degraded: Hub not available, cannot create room")
		metrics.RecordRoomCreation("failed", start)
		return
	}

	code, err := h.hub.CreateRoom(r.Context())
	if err == game.ErrRoomCodeConflict {
		metrics.RecordRoomCreation("failed", start)
		apierror.Conflict("Room code conflict, please retry").Write(w)
		return
	}
	if err != nil {
		slog.Warn("degraded: Hub.CreateRoom failed", "error", err)
		metrics.RecordRoomCreation("failed", start)
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]string{"code": ""},
			"Room creation temporarily unavailable, please retry")
		return
	}

	metrics.RecordRoomCreation("success", start)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}

// CheckRoom handles GET /api/registry/check/{code}
func (h *LobbyHandler) CheckRoom(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		apierror.BadRequest("Room code is required").Write(w)
		return
	}

	if _, err := domain.NewRoomCode(code); err != nil {
		if len(code) != config.RoomCodeLen {
			apierror.BadRequest("invalid room code").Write(w)
		} else {
			apierror.BadRequest("invalid room code charset").Write(w)
		}
		return
	}

	if !RequireHubDegraded(h.hub, w, http.StatusOK,
		map[string]interface{}{
			"code":     code,
			"exists":   false,
			"degraded": true,
		},
		"Room check temporarily unavailable") {
		slog.Warn("degraded: Hub not available, cannot check room")
		return
	}

	info, err := h.hub.CheckRoomCached(r.Context(), code)
	if err != nil {
		slog.Warn("degraded: CheckRoom failed, returning not-found", "code", code, "error", err)
		WriteDegradedJSON(w, http.StatusOK,
			map[string]interface{}{
				"code":     code,
				"exists":   false,
				"degraded": true,
			},
			"Room check temporarily unavailable")
		return
	}

	if info == nil {
		apierror.NotFound("Room not found").Write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Last-Modified", time.Unix(info.CreatedAt, 0).UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":        info.Code,
		"phase":       info.Phase,
		"playerCount": info.PlayerCount,
		"createdAt":   info.CreatedAt,
	})
}

func writeDegradedLobbyList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"lobbies":     []interface{}{},
		"total":       0,
		"has_more":    false,
		"next_cursor": "",
		"degraded":    true,
	})
}

// ListLobbies handles GET /api/registry/lobbies
func (h *LobbyHandler) ListLobbies(w http.ResponseWriter, r *http.Request) {
	limit := config.DefaultPageSize
	cursor := r.URL.Query().Get("cursor")
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= config.MaxPageSize {
			limit = v
		}
	}

	result, err := h.hub.ListLobbies(r.Context(), limit, cursor)
	if err != nil {
		slog.Warn("degraded: returning empty lobby list", "error", err)
		writeDegradedLobbyList(w)
		return
	}

	response := map[string]interface{}{
		"lobbies":     result.Lobbies,
		"total":       result.Total,
		"has_more":    result.HasMore,
		"next_cursor": result.NextCursor,
	}
	bodyBytes, err := json.Marshal(response)
	if err != nil {
		slog.Warn("ListLobbies: failed to marshal response", "error", err)
		writeDegradedLobbyList(w)
		return
	}

	hash := sha256.Sum256(bodyBytes)
	etag := fmt.Sprintf(`"%x"`, hash[:16])

	if match := r.Header.Get("If-None-Match"); match == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(bodyBytes)
}

// MatchRoom handles POST /api/v1/registry/match
func (h *LobbyHandler) MatchRoom(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if !RequireHubDegraded(h.hub, w, http.StatusServiceUnavailable,
		map[string]string{"lobbyCode": ""},
		"Room match temporarily unavailable, please retry") {
		slog.Warn("degraded: Hub not available, cannot match room")
		metrics.RecordRoomCreation("failed", start)
		return
	}

	code, err := h.hub.MatchRoom(r.Context())
	if err == game.ErrRoomCodeConflict {
		metrics.RecordRoomCreation("failed", start)
		apierror.Conflict("Room code conflict, please retry").Write(w)
		return
	}
	if err != nil {
		slog.Warn("degraded: Hub.MatchRoom failed", "error", err)
		metrics.RecordRoomCreation("failed", start)
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]string{"lobbyCode": ""},
			"Room match temporarily unavailable, please retry")
		return
	}

	metrics.RecordRoomCreation("success", start)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"lobbyCode": code})
}
