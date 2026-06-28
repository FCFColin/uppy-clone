package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/metrics"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
)

// WebSocket handles GET /lobby/{code}/ws upgrades for authenticated players.
func (h *LobbyHandler) WebSocket(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		metrics.RecordWSConnection("rejected")
		apierror.BadRequest("Room code is required").Write(w)
		return
	}

	userId, ok := h.authenticateWSRequest(w, r)
	if !ok {
		metrics.RecordWSConnection("rejected")
		return
	}

	if !h.validateWSOrigin(w, r) {
		metrics.RecordWSConnection("rejected")
		return
	}

	room := h.hub.GetRoom(code)
	if room == nil {
		metrics.RecordWSConnection("rejected")
		apierror.NotFound("Room not found").Write(w)
		return
	}

	if !h.reserveWSConnection(w) {
		metrics.RecordWSConnection("rejected")
		return
	}

	conn, ok := h.upgradeWSConnection(w, r)
	if !ok {
		h.hub.DecrementWSConnection()
		metrics.RecordWSConnection("rejected")
		return
	}

	metrics.RecordWSConnection("established")
	h.startWSPumps(room, userId, conn, r.Context())
}

func (h *LobbyHandler) upgradeWSConnection(w http.ResponseWriter, r *http.Request) (*websocket.Conn, bool) {
	reqUpgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(_ *http.Request) bool { return true },
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
