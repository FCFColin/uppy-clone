package handler

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/metrics"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
)

// WebSocket handles GET /lobby/{code}/ws upgrades for authenticated players.
func (h *LobbyHandler) WebSocket(w http.ResponseWriter, r *http.Request) {
	established := false
	defer func() {
		if !established {
			metrics.RecordWSConnection("rejected")
		}
	}()

	code := URLParam(r, "code")
	if code == "" {
		apierror.BadRequest("Room code is required").Write(w)
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
		apierror.NotFound("Room not found").Write(w)
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
	_ = room.RunSession(r.Context(), userId, conn)
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
		apierror.Forbidden("origin not allowed").Write(w)
		return false
	}
	return true
}

func (h *LobbyHandler) reserveWSConnection(w http.ResponseWriter) bool {
	if !h.hub.TryReserveWSConnection() {
		apierror.New(http.StatusServiceUnavailable, "Service Unavailable",
			"WebSocket connection limit reached, please try again later").Write(w)
		return false
	}
	return true
}

func (h *LobbyHandler) authenticateWSRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	userId, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		apierror.Unauthorized("Unauthorized").Write(w)
		return "", false
	}
	return userId, true
}
