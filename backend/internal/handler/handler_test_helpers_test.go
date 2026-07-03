package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
)

func newTestLobbyHandler() *LobbyHandler {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	return NewLobbyHandler(hub, nil)
}

func testAuthMiddleware(userID, nickname string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, nickname)
		next(w, r.WithContext(ctx))
	}
}

func newWSTestServer(h *LobbyHandler, userID, nickname string) *httptest.Server {
	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", testAuthMiddleware(userID, nickname, h.WebSocket))
	return httptest.NewServer(r)
}

func newWSTestServerMultiUser(h *LobbyHandler) *httptest.Server {
	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-Test-User-ID")
		if userID == "" {
			userID = "default-user"
		}
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, "nick")
		h.WebSocket(w, r.WithContext(ctx))
	})
	return httptest.NewServer(r)
}

func wsDial(t *testing.T, server *httptest.Server, code, origin string) (*websocket.Conn, *http.Response) {
	t.Helper()
	url := "ws" + server.URL[4:] + "/lobby/" + code + "/ws"
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	conn, resp, err := dialer.Dial(url, hdr)
	if err != nil && resp == nil {
		t.Fatalf("websocket dial failed (no response): %v", err)
	}
	return conn, resp
}

func newTestLobbyHandlerWithOrigins(origins []string) *LobbyHandler {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	return NewLobbyHandler(hub, origins)
}

func waitForConnCount(h *LobbyHandler, target int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.hub.WSConnCount() == target {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return h.hub.WSConnCount() == target
}

func withChiParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
