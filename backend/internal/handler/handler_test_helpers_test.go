package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func newTestLobbyHandler() *LobbyHandler {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	return NewLobbyHandler(hub, nil)
}

func testAuthMiddleware(userID, nickname string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, nickname)
		next(w, r.WithContext(ctx))
	}
}

func newWSTestServer(h *LobbyHandler, userID, nickname string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lobby/{code}/ws", testAuthMiddleware(userID, nickname, h.WebSocket))
	return httptest.NewServer(mux)
}

func newWSTestServerMultiUser(h *LobbyHandler) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lobby/{code}/ws", func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-Test-User-ID")
		if userID == "" {
			userID = "default-user"
		}
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, "nick")
		h.WebSocket(w, r.WithContext(ctx))
	})
	return httptest.NewServer(mux)
}

func wsDial(t *testing.T, server *httptest.Server, code, origin string) (*websocket.Conn, int) {
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
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
		_ = resp.Body.Close()
	}
	return conn, statusCode
}

func newTestLobbyHandlerWithOrigins(origins []string) *LobbyHandler {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
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

// withPathParam sets a URL path parameter on the request using the Go 1.22+
// standard library mechanism (r.SetPathValue), avoiding chi-specific context
// plumbing in tests. The handler-side URLParam wrapper reads this value.
func withPathParam(r *http.Request, key, val string) *http.Request {
	r.SetPathValue(key, val)
	return r
}

// newTestAdminHandler builds an AdminHandler with a test JWT manager and no
// dependencies. Tests use it as a starting point and inject stores as needed.
func newTestAdminHandler() *AdminHandler {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	return NewAdminHandler(nil, jwtMgr, nil)
}
