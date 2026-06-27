package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/store"
)

func newTestLobbyHandler() *LobbyHandler {
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	return NewLobbyHandler(hub, jwtMgr, nil)
}

func TestCreateRoom(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)

	h.CreateRoom(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] == "" {
		t.Error("expected non-empty room code")
	}
}

func TestCheckRoom_Exists(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	// Create a room first
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)
	h.CreateRoom(w, r)

	var createResp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&createResp)
	code := createResp["code"]

	// Check the room
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/"+code, nil)

	// Use chi router to set URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("code", code)
	r2 = r2.WithContext(context.WithValue(r2.Context(), chi.RouteCtxKey, rctx))

	h.CheckRoom(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", w2.Code, http.StatusOK, w2.Body.String())
	}

	var checkResp map[string]interface{}
	_ = json.NewDecoder(w2.Body).Decode(&checkResp)
	if checkResp["code"] != code {
		t.Errorf("code = %v, want %q", checkResp["code"], code)
	}
}

func TestCheckRoom_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ZZZZZ", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("code", "ZZZZZ")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	h.CheckRoom(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCheckRoom_MissingCode(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("code", "")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	h.CheckRoom(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestMatchRoom(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/registry/match", nil)

	h.MatchRoom(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["lobbyCode"] == "" {
		t.Error("expected non-empty lobbyCode")
	}
}

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

// 企业为何需要：优雅降级响应是防止级联故障的最后防线。
// 响应格式、状态码、Content-Type 必须正确，否则客户端无法正确处理降级状态。

// --- WriteDegradedJSON 写入正确的 JSON 结构 ---

func TestWriteDegradedJSON_Structure(t *testing.T) {
	rec := httptest.NewRecorder()

	data := map[string]string{"room": "ABC12", "players": "3"}
	WriteDegradedJSON(rec, http.StatusOK, data, "Redis unavailable")

	var resp DegradedResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Degraded != true {
		t.Errorf("degraded = %v, want true", resp.Degraded)
	}

	if resp.Message != "Redis unavailable" {
		t.Errorf("message = %q, want %q", resp.Message, "Redis unavailable")
	}

	// Data field should contain the map — decode to verify
	dataBytes, _ := json.Marshal(resp.Data)
	var result map[string]string
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		t.Fatalf("failed to decode data field: %v", err)
	}
	if result["room"] != "ABC12" {
		t.Errorf("data.room = %q, want %q", result["room"], "ABC12")
	}
	if result["players"] != "3" {
		t.Errorf("data.players = %q, want %q", result["players"], "3")
	}
}

// --- Content-Type 必须是 application/json ---

func TestWriteDegradedJSON_ContentType(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteDegradedJSON(rec, http.StatusOK, nil, "")

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

// --- 状态码必须正确传递 ---

func TestWriteDegradedJSON_StatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"202 Accepted", http.StatusAccepted},
		{"206 Partial Content", http.StatusPartialContent},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
		{"502 Bad Gateway", http.StatusBadGateway},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"429 Too Many Requests", http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteDegradedJSON(rec, tt.status, nil, "degraded")

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
		})
	}
}

// --- message omitempty：空字符串时 message 字段不应出现在 JSON 中 ---

func TestWriteDegradedJSON_MessageOmitempty(t *testing.T) {
	t.Run("empty message omitted", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "")

		var raw map[string]json.RawMessage
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if _, exists := raw["message"]; exists {
			t.Errorf("message field should be omitted when empty, but got: %s", raw["message"])
		}
	})

	t.Run("non-empty message present", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "cache miss")

		var raw map[string]json.RawMessage
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		msgRaw, exists := raw["message"]
		if !exists {
			t.Fatal("message field should be present when non-empty")
		}

		var msg string
		if err := json.Unmarshal(msgRaw, &msg); err != nil {
			t.Fatalf("failed to unmarshal message: %v", err)
		}
		if msg != "cache miss" {
			t.Errorf("message = %q, want %q", msg, "cache miss")
		}
	})
}

// --- degraded 字段始终为 true ---

func TestWriteDegradedJSON_DegradedAlwaysTrue(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		data    interface{}
		message string
	}{
		{"200 with data", http.StatusOK, "hello", "msg"},
		{"503 with nil data", http.StatusServiceUnavailable, nil, ""},
		{"500 with empty data", http.StatusInternalServerError, "", ""},
		{"200 with slice", http.StatusOK, []int{1, 2, 3}, "partial"},
		{"200 with map", http.StatusOK, map[string]int{"a": 1}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteDegradedJSON(rec, tt.status, tt.data, tt.message)

			var resp DegradedResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}

			if !resp.Degraded {
				t.Errorf("degraded = false, want true for %s", tt.name)
			}
		})
	}
}

// --- data 字段始终存在（即使为 nil） ---

func TestWriteDegradedJSON_DataAlwaysPresent(t *testing.T) {
	t.Run("nil data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "")

		var raw map[string]json.RawMessage
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if _, exists := raw["data"]; !exists {
			t.Error("data field should always be present, even when nil")
		}
	})

	t.Run("string data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, "partial data", "")

		var resp DegradedResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		var dataStr string
		dataBytes, _ := json.Marshal(resp.Data)
		if err := json.Unmarshal(dataBytes, &dataStr); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}
		if dataStr != "partial data" {
			t.Errorf("data = %q, want %q", dataStr, "partial data")
		}
	})

	t.Run("slice data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, []string{"a", "b"}, "")

		var resp DegradedResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		var dataSlice []string
		dataBytes, _ := json.Marshal(resp.Data)
		if err := json.Unmarshal(dataBytes, &dataSlice); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}
		if len(dataSlice) != 2 || dataSlice[0] != "a" || dataSlice[1] != "b" {
			t.Errorf("data = %v, want [a, b]", dataSlice)
		}
	})
}

// --- Require* 降级守卫函数 ---

func TestRequireDB_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	// We can't call CreateUser/GetUserByID without a real DB, but we test
	// the nil guard (always reachable) and the non-nil path (always true).
	t.Run("nil db returns false", func(t *testing.T) {
		w := httptest.NewRecorder()
		result := RequireDB(nil, w)
		if result {
			t.Error("RequireDB(nil) = true, want false")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("non-nil db returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		// Use a non-nil *PostgresStore pointer (zero value)
		var zeroStore store.PostgresStore
		result := RequireDB(&zeroStore, w)
		if !result {
			t.Error("RequireDB(non-nil) = false, want true")
		}
	})
}

func TestRequireRedis_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	t.Run("nil redis returns false", func(t *testing.T) {
		w := httptest.NewRecorder()
		result := RequireRedis(nil, w)
		if result {
			t.Error("RequireRedis(nil) = true, want false")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("non-nil redis returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		var zeroStore store.RedisStore
		result := RequireRedis(&zeroStore, w)
		if !result {
			t.Error("RequireRedis(non-nil) = false, want true")
		}
	})
}

func TestRequireHub_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	t.Run("nil hub returns false", func(t *testing.T) {
		w := httptest.NewRecorder()
		result := RequireHub(nil, w)
		if result {
			t.Error("RequireHub(nil) = true, want false")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}

		var resp apierror.ProblemDetails
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if resp.Status != http.StatusServiceUnavailable {
			t.Errorf("error status = %d, want %d", resp.Status, http.StatusServiceUnavailable)
		}
	})

	t.Run("non-nil hub returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
		result := RequireHub(hub, w)
		if !result {
			t.Error("RequireHub(non-nil) = false, want true")
		}
	})
}

func TestRequireHubDegraded_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	t.Run("nil hub returns false with degraded JSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		payload := map[string]string{"status": "degraded"}
		result := RequireHubDegraded(nil, w, http.StatusOK, payload, "Hub unavailable")
		if result {
			t.Error("RequireHubDegraded(nil) = true, want false")
		}

		var resp DegradedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if !resp.Degraded {
			t.Error("degraded = false, want true")
		}
		if resp.Message != "Hub unavailable" {
			t.Errorf("message = %q, want %q", resp.Message, "Hub unavailable")
		}
	})

	t.Run("non-nil hub returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
		result := RequireHubDegraded(hub, w, http.StatusOK, nil, "")
		if !result {
			t.Error("RequireHubDegraded(non-nil) = false, want true")
		}
	})
}

// --- 完整 JSON 输出验证 ---

func TestWriteDegradedJSON_FullOutput(t *testing.T) {
	t.Run("with message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusPartialContent, "partial", "cache degraded")

		body := rec.Body.String()
		if !strings.Contains(body, `"degraded":true`) {
			t.Errorf("body should contain degraded:true, got: %s", body)
		}
		if !strings.Contains(body, `"message":"cache degraded"`) {
			t.Errorf("body should contain message, got: %s", body)
		}
		if !strings.Contains(body, `"data":"partial"`) {
			t.Errorf("body should contain data, got: %s", body)
		}
	})

	t.Run("without message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "")

		body := rec.Body.String()
		if !strings.Contains(body, `"degraded":true`) {
			t.Errorf("body should contain degraded:true, got: %s", body)
		}
		if strings.Contains(body, `"message"`) {
			t.Errorf("body should NOT contain message field when empty, got: %s", body)
		}
		if !strings.Contains(body, `"data":null`) {
			t.Errorf("body should contain data:null for nil data, got: %s", body)
		}
	})
}

func withChiParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestCheckRoom_InvalidCharset(t *testing.T) {
	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABCD0", nil), "code", "ABCD0")
	h.CheckRoom(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCheckRoom_InvalidLength(t *testing.T) {
	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABC", nil), "code", "ABC")
	h.CheckRoom(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRefreshToken_NilRedis_Degraded(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"some-token"}`))
	r.Header.Set("Content-Type", "application/json")
	h.RefreshToken(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 degraded", w.Code)
	}
}

func TestExportUserData_Unauthorized(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	h.ExportUserData(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteUserData_Unauthorized(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	h.DeleteUserData(w, httptest.NewRequest(http.MethodDelete, "/api/v1/user/data", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestLogout_InvalidBody(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader("{bad"))
	r.Header.Set("Content-Type", "application/json")
	h.Logout(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateRoom_NilHub_Degraded(t *testing.T) {
	h := &LobbyHandler{hub: nil}
	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestMatchRoom_NilHub_Degraded(t *testing.T) {
	h := &LobbyHandler{hub: nil}
	w := httptest.NewRecorder()
	h.MatchRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/match", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
