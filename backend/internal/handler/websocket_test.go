package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── WebSocket integration test helpers ──────────────────────────────
//
// These tests verify the WebSocket handler (LobbyHandler.WebSocket) end-to-end
// using httptest.NewServer + gorilla/websocket.Dialer. The Hub is created with
// nil store/redis so tests run without external dependencies.
//
// Authentication is injected via auth.WithAuthenticatedUser (the inverse of
// auth.GetAuthenticatedUser) to bypass JWT cookie validation. Origin validation
// is exercised by setting the Origin header on the dialer.

// testAuthMiddleware injects an authenticated user into the request context,
// bypassing JWT cookie validation. This is the public inverse of AuthMiddleware.
func testAuthMiddleware(userID, nickname string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, nickname)
		next(w, r.WithContext(ctx))
	}
}

// newWSTestServer creates an httptest.Server with a chi router that routes
// /lobby/{code}/ws to the LobbyHandler's WebSocket method, with test auth
// injected for the given user.
func newWSTestServer(h *LobbyHandler, userID, nickname string) *httptest.Server {
	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", testAuthMiddleware(userID, nickname, h.WebSocket))
	return httptest.NewServer(r)
}

// newWSTestServerMultiUser creates a test server that reads the user ID from
// the X-Test-User-ID request header, allowing multiple distinct users to
// connect in the same test (required for concurrent-connection tests).
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

// wsDial connects to the given server's /lobby/{code}/ws endpoint with the
// specified Origin header. Returns the WebSocket connection and HTTP response.
// On failure, returns a nil conn and the HTTP response (if any).
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

// newTestLobbyHandlerWithOrigins creates a LobbyHandler with the given allowed
// origins and a Hub backed by nil store/redis (no external deps).
func newTestLobbyHandlerWithOrigins(origins []string) *LobbyHandler {
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	return NewLobbyHandler(hub, jwtMgr, origins)
}

// waitForConnCount polls hub.WSConnCount until it reaches the target or the
// deadline elapses. Returns true if the target was reached.
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

// ─── TestWebSocket_ConnectAndDisconnect ──────────────────────────────
//
// Verifies that a WebSocket client can connect to a valid room and that the
// connection counter is cleaned up on disconnect.

func TestWebSocket_ConnectAndDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	conn, resp := wsDial(t, server, code, "http://localhost")
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}

	// Verify WS connection count incremented after the server-side pumps start.
	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("expected 1 WS connection, got %d", h.hub.WSConnCount())
	}

	// Close client connection — server readPump should detect the close and
	// decrement the counter.
	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close failed: %v", err)
	}

	if !waitForConnCount(h, 0, 3*time.Second) {
		t.Fatalf("expected 0 WS connections after disconnect, got %d", h.hub.WSConnCount())
	}
}

// ─── TestWebSocket_RoomCreation ──────────────────────────────────────
//
// Verifies that a client connecting to a room receives the initial room state
// (snapshot) message. HandleJoin → notifyJoin → sendToPlayer(snapshot) is the
// first message queued on the player's Send channel.

func TestWebSocket_RoomCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer conn.Close()

	// Set a read deadline to avoid blocking forever if the server doesn't send.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	// The first message should be a snapshot (MsgSnapshot = 0x01).
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if len(msg) == 0 {
		t.Fatal("expected non-empty message")
	}
	if msg[0] != protocol.MsgSnapshot {
		t.Fatalf("expected first byte MsgSnapshot (0x%02x), got 0x%02x", protocol.MsgSnapshot, msg[0])
	}
}

// ─── TestWebSocket_ConcurrentConnections ─────────────────────────────
//
// Verifies that multiple WebSocket clients can connect concurrently and that
// hub.WSConnCount() remains accurate. Run with -race to detect data races.

func TestWebSocket_ConcurrentConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServerMultiUser(h)
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	const N = 10
	var wg sync.WaitGroup
	conns := make([]*websocket.Conn, N)
	errors := make([]error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			url := "ws" + server.URL[4:] + "/lobby/" + code + "/ws"
			dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
			hdr := http.Header{}
			hdr.Set("Origin", "http://localhost")
			hdr.Set("X-Test-User-ID", fmt.Sprintf("user%d", idx))
			conn, _, err := dialer.Dial(url, hdr)
			if err != nil {
				errors[idx] = err
				return
			}
			conns[idx] = conn
		}(i)
	}
	wg.Wait()

	// Report any dial errors
	for i, e := range errors {
		if e != nil {
			t.Errorf("dial %d failed: %v", i, e)
		}
	}
	if t.Failed() {
		for _, c := range conns {
			if c != nil {
				c.Close()
			}
		}
		return
	}

	// Wait for all server-side pumps to start and increment the counter.
	if !waitForConnCount(h, N, 5*time.Second) {
		t.Fatalf("expected %d WS connections, got %d", N, h.hub.WSConnCount())
	}

	// Close all connections concurrently
	var closeWg sync.WaitGroup
	for _, c := range conns {
		closeWg.Add(1)
		go func(c *websocket.Conn) {
			defer closeWg.Done()
			c.Close()
		}(c)
	}
	closeWg.Wait()

	// Wait for server to clean up all connections
	if !waitForConnCount(h, 0, 5*time.Second) {
		t.Fatalf("expected 0 WS connections after cleanup, got %d", h.hub.WSConnCount())
	}
}

// ─── TestWebSocket_InvalidRoom ───────────────────────────────────────
//
// Verifies that connecting to a non-existent room is rejected with a 404 Not
// Found response (before the WebSocket upgrade happens).

func TestWebSocket_InvalidRoom(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	// Don't create any rooms; try to connect to a non-existent code.
	conn, resp := wsDial(t, server, "NOPE1", "http://localhost")
	if conn != nil {
		conn.Close()
		t.Fatal("expected connection to be rejected (nil conn)")
	}
	if resp == nil {
		t.Fatal("expected HTTP response for rejected connection")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", resp.StatusCode)
	}
}

// ─── TestWebSocket_Unauthorized ──────────────────────────────────────
//
// Verifies that a WebSocket request without authentication is rejected with
// 401 Unauthorized. This is a security-critical test: the WS handler must not
// upgrade the connection before checking authentication.

func TestWebSocket_Unauthorized(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	// Create a server WITHOUT the test auth middleware — simulates an
	// unauthenticated request.
	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", h.WebSocket)
	server := httptest.NewServer(r)
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	conn, resp := wsDial(t, server, code, "http://localhost")
	if conn != nil {
		conn.Close()
		t.Fatal("expected connection to be rejected (nil conn)")
	}
	if resp == nil {
		t.Fatal("expected HTTP response for rejected connection")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}
}

// ─── TestWebSocket_ForbiddenOrigin ───────────────────────────────────
//
// Verifies that a WebSocket request with a disallowed Origin is rejected with
// 403 Forbidden (CSWSH protection).

func TestWebSocket_ForbiddenOrigin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Use an Origin that doesn't match the allowed list.
	conn, resp := wsDial(t, server, code, "http://evil.example.com")
	if conn != nil {
		conn.Close()
		t.Fatal("expected connection to be rejected (nil conn)")
	}
	if resp == nil {
		t.Fatal("expected HTTP response for rejected connection")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", resp.StatusCode)
	}
}

// ─── TestWebSocket_RateLimit ─────────────────────────────────────────
//
// Verifies that the WebSocket bulkhead (global connection limit) rejects new
// connections when the limit is reached. This is the DoS defense described in
// the Hub's CanAcceptWSConnection method.

func TestWebSocket_RateLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a handler with a very low WS connection limit (2).
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 2, 50, nil)
	h := NewLobbyHandler(hub, jwtMgr, []string{"http://localhost"})
	server := newWSTestServerMultiUser(h)
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Connect 2 clients to fill the limit.
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr1 := http.Header{}
	hdr1.Set("Origin", "http://localhost")
	hdr1.Set("X-Test-User-ID", "user0")
	conn1, _, err := dialer.Dial("ws"+server.URL[4:]+"/lobby/"+code+"/ws", hdr1)
	if err != nil {
		t.Fatalf("dial 1 failed: %v", err)
	}
	defer conn1.Close()

	hdr2 := http.Header{}
	hdr2.Set("Origin", "http://localhost")
	hdr2.Set("X-Test-User-ID", "user1")
	conn2, _, err := dialer.Dial("ws"+server.URL[4:]+"/lobby/"+code+"/ws", hdr2)
	if err != nil {
		t.Fatalf("dial 2 failed: %v", err)
	}
	defer conn2.Close()

	// Wait for both server-side pumps to start.
	if !waitForConnCount(h, 2, 3*time.Second) {
		t.Fatalf("expected 2 WS connections, got %d", hub.WSConnCount())
	}

	// The 3rd connection should be rejected with 503 Service Unavailable.
	hdr3 := http.Header{}
	hdr3.Set("Origin", "http://localhost")
	hdr3.Set("X-Test-User-ID", "user2")
	_, resp3, err := dialer.Dial("ws"+server.URL[4:]+"/lobby/"+code+"/ws", hdr3)
	if err == nil {
		t.Fatal("expected dial 3 to fail (rate limit)")
	}
	if resp3 == nil {
		t.Fatal("expected HTTP response for rejected connection")
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", resp3.StatusCode)
	}
}
