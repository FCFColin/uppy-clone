package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
)

func TestWebSocket_MissingRoomCode(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins(nil)
	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby//ws", nil), "code", "")
	h.WebSocket(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestWebSocket_UnauthorizedShort(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/ABCDE/ws", nil), "code", "ABCDE")
	h.WebSocket(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestWebSocket_ForbiddenOriginShort(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/ABCDE/ws", nil), "code", "ABCDE")
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "nick"))
	r.Header.Set("Origin", "http://evil.example")
	h.WebSocket(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestWebSocket_RateLimitShort(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 1, 50)
	hub.IncrementWSConnection()
	h := NewLobbyHandler(hub, []string{"http://localhost"})
	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/"+code+"/ws", nil), "code", code)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "nick"))
	r.Header.Set("Origin", "http://localhost")
	h.WebSocket(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestWebSocket_RoomNotFoundShort(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/ABCDE/ws", nil), "code", "ABCDE")
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "nick"))
	r.Header.Set("Origin", "http://localhost")
	h.WebSocket(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestWebSocket_UpgradeFailsWithoutHijacker(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/"+code+"/ws", nil), "code", code)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "nick"))
	r.Header.Set("Origin", "http://localhost")
	h.WebSocket(w, r)
	// httptest.ResponseRecorder is not a Hijacker; upgrade should fail silently.
	if w.Code == http.StatusSwitchingProtocols {
		t.Error("expected upgrade to fail on non-Hijacker recorder")
	}
	if h.hub.WSConnCount() != 0 {
		t.Fatalf("WSConnCount = %d after failed upgrade, want 0", h.hub.WSConnCount())
	}
}

func TestAuthenticateWSRequest(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := h.authenticateWSRequest(w, r); ok {
		t.Error("expected unauthenticated request to fail")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2 = r2.WithContext(auth.WithAuthenticatedUser(r2.Context(), "uid", "nick"))
	id, ok := h.authenticateWSRequest(w2, r2)
	if !ok || id != "uid" {
		t.Errorf("authenticateWSRequest = (%q, %v), want (uid, true)", id, ok)
	}
}

func TestValidateWSOrigin(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://evil.example")
	if h.validateWSOrigin(w, r) {
		t.Error("disallowed origin should fail validation")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("Origin", "http://localhost")
	if !h.validateWSOrigin(w2, r2) {
		t.Error("allowed origin should pass validation")
	}
}

func TestReserveWSConnection(t *testing.T) {
	t.Parallel()

	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 1, 50)
	hub.IncrementWSConnection()
	h := NewLobbyHandler(hub, nil)

	w := httptest.NewRecorder()
	if h.reserveWSConnection(w) {
		t.Error("at-capacity hub should reject")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	hub2 := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 10, 50)
	h2 := NewLobbyHandler(hub2, nil)
	w2 := httptest.NewRecorder()
	if !h2.reserveWSConnection(w2) {
		t.Error("hub with capacity should reserve")
	}
	hub2.DecrementWSConnection()
}

func TestStartWSPumps_ConnectAndDisconnect(t *testing.T) {
	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, statusCode := wsDial(t, server, code, "http://localhost")
	if statusCode != 0 && statusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", statusCode)
	}
	if conn == nil {
		t.Fatal("expected connection")
	}
	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}
	_ = conn.Close()
	if !waitForConnCount(h, 0, 3*time.Second) {
		t.Fatalf("after close conn count = %d", h.hub.WSConnCount())
	}
}

func TestStartWSPumps_JoinFails(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 10, 1)
	h := NewLobbyHandler(hub, []string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room := hub.GetRoom(code)
	if err := room.HandleJoin("existing", nil); err != nil {
		t.Fatal(err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer func() { _ = conn.Close() }()
	time.Sleep(100 * time.Millisecond)
	if hub.WSConnCount() != 0 {
		t.Fatalf("join failure should decrement WS count, got %d", hub.WSConnCount())
	}
}

func TestWebSocket_ReadPumpUnexpectedClose(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 0, 0)
	h := NewLobbyHandler(hub, []string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer func() { _ = conn.Close() }()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	err = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseAbnormalClosure, "unexpected"),
		time.Now().Add(time.Second),
	)
	if err != nil {
		t.Fatalf("WriteControl: %v", err)
	}

	if !waitForConnCount(h, 0, 3*time.Second) {
		t.Fatalf("conn count = %d after abnormal close", h.hub.WSConnCount())
	}
}

// TestStartWSPumps_HandlerTimeout handler-028: verifies the handler-level
// timeout caps the maximum WebSocket session duration, closing zombie
// connections that would otherwise hang indefinitely.
func TestStartWSPumps_HandlerTimeout(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSHandlerTimeout = 200 * time.Millisecond
	timeouts.WSPingInterval = time.Hour // isolate handler timeout from ping/pong cycle
	hub := game.NewHub(nil, nil, timeouts, 0, 0)
	h := NewLobbyHandler(hub, []string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer func() { _ = conn.Close() }()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d, want 1 before handler timeout", h.hub.WSConnCount())
	}

	// handler-028: after WSHandlerTimeout fires, wsCtx is cancelled, writePump
	// closes the connection, readPump unblocks, and the WS count drops to 0.
	if !waitForConnCount(h, 0, 3*time.Second) {
		t.Fatalf("conn count = %d after handler timeout, want 0", h.hub.WSConnCount())
	}
}
