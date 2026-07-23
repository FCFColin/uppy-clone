package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/metrics"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
)

// jsonMarshalFn is replaceable in unit tests (ListLobbies response encoding).
var jsonMarshalFn = json.Marshal

type registryRoomParams struct {
	emptyKey      string
	emptyVal      string
	unavailMsg    string
	unavailLog    string
	failLog       string
	degradedMsg   string
	responseField string
}

type registryRoomFn func(context.Context) (string, error)

func (h *LobbyHandler) handleRegistryRoom(w http.ResponseWriter, r *http.Request, p registryRoomParams, op registryRoomFn) {
	start := time.Now()
	emptyResp := map[string]string{p.emptyKey: p.emptyVal}
	if !RequireHubDegraded(h.hub, w, http.StatusServiceUnavailable, emptyResp, p.unavailMsg) {
		slog.Warn("degraded: " + p.unavailLog)
		metrics.RecordRoomCreation("failed", start)
		return
	}

	code, err := op(r.Context())
	if err == game.ErrRoomCodeConflict {
		metrics.RecordRoomCreation("failed", start)
		domain.Conflict("Room code conflict, please retry").Write(w)
		return
	}
	if err != nil {
		slog.Warn("degraded: "+p.failLog, "error", err)
		metrics.RecordRoomCreation("failed", start)
		WriteDegradedJSON(w, http.StatusServiceUnavailable, emptyResp, p.degradedMsg)
		return
	}

	metrics.RecordRoomCreation("success", start)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{p.responseField: code})
}

// CreateRoom handles POST /api/registry/create
func (h *LobbyHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	h.handleRegistryRoom(w, r, registryRoomParams{
		emptyKey:      codeKey,
		emptyVal:      "",
		unavailMsg:    "Room service temporarily unavailable, please retry",
		unavailLog:    "Hub not available, cannot create room",
		failLog:       "Hub.CreateRoom failed",
		degradedMsg:   "Room creation temporarily unavailable, please retry",
		responseField: codeKey,
	}, func(ctx context.Context) (string, error) {
		return h.hub.CreateRoom(ctx)
	})
}

// CheckRoom handles GET /api/registry/check/{code}
func (h *LobbyHandler) CheckRoom(w http.ResponseWriter, r *http.Request) {
	code := URLParam(r, codeKey)
	if code == "" {
		domain.BadRequest("Room code is required").Write(w)
		return
	}

	if _, err := domain.NewRoomCode(code); err != nil {
		if len(code) != config.RoomCodeLen {
			domain.BadRequest("invalid room code").Write(w)
		} else {
			domain.BadRequest("invalid room code charset").Write(w)
		}
		return
	}

	if !RequireHubDegraded(h.hub, w, http.StatusServiceUnavailable,
		map[string]interface{}{
			codeKey:     code,
			"exists":    false,
			degradedKey: true,
		},
		"Room check temporarily unavailable") {
		slog.Warn("degraded: Hub not available, cannot check room")
		return
	}

	info, err := h.hub.CheckRoomCached(r.Context(), code)
	if err != nil {
		// handler-021: Return 503 (Service Unavailable) when Redis is down,
		// not 500 or 404. This signals degradation to the client.
		slog.Warn("degraded: CheckRoom failed, returning not-found", codeKey, code, "error", err)
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]interface{}{
				codeKey:     code,
				"exists":    false,
				degradedKey: true,
			},
			"Room check temporarily unavailable")
		return
	}

	if info == nil {
		domain.NotFound("Room not found").Write(w)
		return
	}

	w.Header().Set("Last-Modified", time.Unix(info.CreatedAt, 0).UTC().Format(http.TimeFormat))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		codeKey:       info.Code,
		"phase":       info.Phase,
		"playerCount": info.PlayerCount,
		"createdAt":   info.CreatedAt,
	})
}

func writeDegradedLobbyList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"lobbies":     []interface{}{},
		"total":       0,
		"has_more":    false,
		"next_cursor": "",
		degradedKey:   true,
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

	result, err := h.hub.ListLobbiesCached(r.Context(), limit, cursor)
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
	bodyBytes, err := jsonMarshalFn(response)
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
	if _, err := w.Write(bodyBytes); err != nil {
		slog.Warn("ListLobbies: failed to write response", "error", err)
	}
}

// MatchRoom handles POST /api/v1/registry/match
func (h *LobbyHandler) MatchRoom(w http.ResponseWriter, r *http.Request) {
	h.handleRegistryRoom(w, r, registryRoomParams{
		emptyKey:      "lobbyCode",
		emptyVal:      "",
		unavailMsg:    "Room match temporarily unavailable, please retry",
		unavailLog:    "Hub not available, cannot match room",
		failLog:       "Hub.MatchRoom failed",
		degradedMsg:   "Room match temporarily unavailable, please retry",
		responseField: "lobbyCode",
	}, func(ctx context.Context) (string, error) {
		return h.hub.MatchRoom(ctx)
	})
}

// WebSocket handles GET /lobby/{code}/ws upgrades for authenticated players.
func (h *LobbyHandler) WebSocket(w http.ResponseWriter, r *http.Request) {
	established := false
	defer func() {
		if !established {
			metrics.RecordWSConnection("rejected")
		}
	}()

	code := URLParam(r, codeKey)
	if code == "" {
		domain.BadRequest("Room code is required").Write(w)
		return
	}

	userId, ok := h.authenticateWSRequest(w, r)
	if !ok {
		return
	}

	if !h.validateWSOrigin(w, r) {
		return
	}

	room := h.hub.GetRoom(code)
	if room == nil {
		domain.NotFound("Room not found").Write(w)
		return
	}

	if !h.reserveWSConnection(w) {
		return
	}

	conn, ok := h.upgradeWSConnection(w, r)
	if !ok {
		h.hub.DecrementWSConnection()
		return
	}

	established = true
	metrics.RecordWSConnection("established")
	_ = game.NewWSSession(room).RunSession(r.Context(), userId, conn)
	h.hub.DecrementWSConnection()
}

func (h *LobbyHandler) upgradeWSConnection(w http.ResponseWriter, r *http.Request) (*websocket.Conn, bool) {
	reqUpgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return appMiddleware.IsOriginAllowed(r.Header.Get("Origin"), h.allowedOrigins)
		},
	}
	conn, err := reqUpgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return nil, false
	}
	return conn, true
}

func (h *LobbyHandler) validateWSOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if !appMiddleware.IsOriginAllowed(origin, h.allowedOrigins) {
		if origin != "" {
			h.logger.Warn("CSWSH blocked", "origin", origin)
		}
		domain.Forbidden("origin not allowed").Write(w)
		return false
	}
	return true
}

func (h *LobbyHandler) reserveWSConnection(w http.ResponseWriter) bool {
	if !h.hub.TryReserveWSConnection() {
		domain.ServiceUnavailable("WebSocket connection limit reached, please try again later").Write(w)
		return false
	}
	return true
}

func (h *LobbyHandler) authenticateWSRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	userId, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		domain.Unauthorized("Unauthorized").Write(w)
		return "", false
	}
	return userId, true
}
