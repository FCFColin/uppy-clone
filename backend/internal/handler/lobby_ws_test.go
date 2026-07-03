package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestWebSocket_MissingRoomCode(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins(nil)
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby//ws", nil), "code", "")
	h.WebSocket(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestWebSocket_UnauthorizedShort(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/ABCDE/ws", nil), "code", "ABCDE")
	h.WebSocket(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestWebSocket_ForbiddenOriginShort(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/ABCDE/ws", nil), "code", "ABCDE")
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "nick"))
	r.Header.Set("Origin", "http://evil.example")
	h.WebSocket(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestWebSocket_RateLimitShort(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 1, 50, nil)
	hub.IncrementWSConnection()
	h := NewLobbyHandler(hub, []string{"http://localhost"})
	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/"+code+"/ws", nil), "code", code)
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
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/ABCDE/ws", nil), "code", "ABCDE")
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
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/lobby/"+code+"/ws", nil), "code", code)
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

	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 1, 50, nil)
	hub.IncrementWSConnection()
	h := NewLobbyHandler(hub, nil)

	w := httptest.NewRecorder()
	if h.reserveWSConnection(w) {
		t.Error("at-capacity hub should reject")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	hub2 := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 10, 50, nil)
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

	conn, resp := wsDial(t, server, code, "http://localhost")
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
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

func TestMaybeStartReadSpan(t *testing.T) {
	h := newTestLobbyHandler()
	room := h.hub.GetRoom("NONE")
	if room == nil {
		code, _ := h.hub.CreateRoom(context.Background())
		room = h.hub.GetRoom(code)
	}
	var counter uint64
	if span := h.maybeStartReadSpan(context.Background(), room, "p1", protocol.MsgPing, &counter); span != nil {
		t.Fatal("ping should not create span")
	}
	if span := h.maybeStartReadSpan(context.Background(), room, "p1", protocol.MsgTap, &counter); span != nil {
		t.Fatal("first tap should not create span")
	}
	counter = 99
	if span := h.maybeStartReadSpan(context.Background(), room, "p1", protocol.MsgTap, &counter); span == nil {
		t.Fatal("100th tap should create span")
	}
	span := h.maybeStartReadSpan(context.Background(), room, "p1", protocol.MsgSetNickname, &counter)
	if span == nil {
		t.Fatal("set_nickname should create span")
	}
	span.End()
}

func TestWebSocket_PingMessage(t *testing.T) {
	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	// Drain initial snapshot
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	ping := []byte{protocol.MsgPing}
	if err := conn.WriteMessage(websocket.BinaryMessage, ping); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocket_SetNicknameMessage(t *testing.T) {
	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	nick := "Alice"
	payload := append([]byte{byte(len(nick))}, []byte(nick)...)
	msg := append([]byte{protocol.MsgSetNickname}, payload...)
	if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatalf("write set nickname: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocket_TapMessage(t *testing.T) {
	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	tap := []byte{protocol.MsgTap, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // tap near center
	if err := conn.WriteMessage(websocket.BinaryMessage, tap); err != nil {
		t.Fatalf("write tap: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestMaybeStartReadSpan_RestartVoteAndUnknown(t *testing.T) {
	h := newTestLobbyHandler()
	code, _ := h.hub.CreateRoom(context.Background())
	room := h.hub.GetRoom(code)

	if span := h.maybeStartReadSpan(context.Background(), room, "p1", protocol.MsgRestartVote, new(uint64)); span == nil {
		t.Fatal("restart_vote should create span")
	} else {
		span.End()
	}
	if span := h.maybeStartReadSpan(context.Background(), room, "p1", 0xFF, new(uint64)); span == nil {
		t.Fatal("unknown message should create span")
	} else {
		span.End()
	}
}

func TestWebSocket_WritePumpPing(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSPingInterval = 50 * time.Millisecond
	hub := game.NewHub(nil, nil, timeouts, 0, 0, nil)
	h := NewLobbyHandler(hub, []string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	time.Sleep(150 * time.Millisecond)
}

func TestStartWSPumps_JoinFails(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 10, 1, nil)
	h := NewLobbyHandler(hub, []string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room := hub.GetRoom(code)
	room.HandleJoin("existing", nil)

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)
	if hub.WSConnCount() != 0 {
		t.Fatalf("join failure should decrement WS count, got %d", hub.WSConnCount())
	}
}

func TestWritePump_ClosedSendChannel(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSPingInterval = time.Hour
	hub := game.NewHub(nil, nil, timeouts, 10, 8, nil)
	h := NewLobbyHandler(hub, nil)
	room := game.NewRoom("PUMP1", hub, nil, timeouts, 4)
	send := make(chan []byte)
	close(send)
	room.HandleJoin("p1", nil)
	pc := room.GetConnection("p1")
	if pc != nil {
		pc.Send = send
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		h.writePump(room, "p1", c, ctx)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
}

func TestStartWSPumps_HandleJoinFailure(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 10, 1, nil)
	h := NewLobbyHandler(hub, nil)
	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room := hub.GetRoom(code)
	if err := room.HandleJoin("existing", nil); err != nil {
		t.Fatalf("HandleJoin: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = hub.TryReserveWSConnection()
		h.startWSPumps(room, "user2", c, r.Context())
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
}

func TestWritePump_NilPlayerConnection(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 10, 8, nil)
	h := NewLobbyHandler(hub, nil)
	room := game.NewRoom("PUMP2", hub, nil, timeouts, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		h.writePump(room, "missing-player", c, ctx)
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
}

func TestWritePump_WriteMessageError(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSPingInterval = time.Hour
	hub := game.NewHub(nil, nil, timeouts, 10, 8, nil)
	h := NewLobbyHandler(hub, nil)
	room := game.NewRoom("PUMP3", hub, nil, timeouts, 4)
	room.HandleJoin("p1", nil)
	pc := room.GetConnection("p1")
	if pc == nil {
		t.Fatal("expected player connection")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go h.writePump(room, "p1", c, ctx)
		time.Sleep(20 * time.Millisecond)
		select {
		case pc.Send <- []byte{protocol.MsgSnapshot, 0x01}:
		default:
			t.Fatal("failed to enqueue broadcast")
		}
		time.Sleep(50 * time.Millisecond)
		_ = c.Close()
		time.Sleep(50 * time.Millisecond)
		cancel()
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
}

func TestWebSocket_EmptyMessage(t *testing.T) {
	h := newTestLobbyHandlerWithOrigins([]string{"http://localhost"})
	server := newWSTestServer(h, "user1", "nick1")
	defer server.Close()

	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, _ := wsDial(t, server, code, "http://localhost")
	if conn == nil {
		t.Fatal("expected connection")
	}
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{}); err != nil {
		t.Fatalf("write empty message: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocket_PongHandler(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSPingInterval = 30 * time.Millisecond
	hub := game.NewHub(nil, nil, timeouts, 0, 0, nil)
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
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	conn.SetPingHandler(nil)
	time.Sleep(200 * time.Millisecond)
}

func TestMaybeStartReadSpan_PingCaseLabel(t *testing.T) {
	h := newTestLobbyHandler()
	code, _ := h.hub.CreateRoom(context.Background())
	room := h.hub.GetRoom(code)
	counter := uint64(0)
	span := h.maybeStartReadSpan(context.Background(), room, "p1", protocol.MsgPing, &counter)
	if span != nil {
		t.Fatal("ping should not create span")
	}
}

func TestWebSocket_ReadPumpUnexpectedClose(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 0, 0, nil)
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
	defer conn.Close()

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

func TestWebSocket_ReadPumpPongHandler(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSPingInterval = 30 * time.Millisecond
	hub := game.NewHub(nil, nil, timeouts, 0, 0, nil)
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
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 5; i++ {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func TestWebSocket_ReadPumpHandleMessageError(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 0, 0, nil)
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
	defer conn.Close()

	if !waitForConnCount(h, 1, 3*time.Second) {
		t.Fatalf("conn count = %d", h.hub.WSConnCount())
	}

	// Invalid tap payload triggers HandleMessage error in readPump.
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{protocol.MsgTap, 0x01}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
}

func TestReadPump_HandleMessageErrorWithSpan(t *testing.T) {
	prev := handleMessageFn
	handleMessageFn = func(_ *game.Room, _ string, _ byte, _ []byte) error {
		return errors.New("handle failed")
	}
	t.Cleanup(func() { handleMessageFn = prev })

	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 10, 8, nil)
	h := NewLobbyHandler(hub, nil)
	room := game.NewRoom("RSPN", hub, nil, timeouts, 4)
	room.HandleJoin("p1", nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		h.readPump(room, "p1", c, ctx, cancel)
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{protocol.MsgSetNickname, 'x'}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
}

func TestWritePump_PingWriteError(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSPingInterval = 10 * time.Millisecond
	hub := game.NewHub(nil, nil, timeouts, 10, 8, nil)
	h := NewLobbyHandler(hub, nil)
	room := game.NewRoom("PING1", hub, nil, timeouts, 4)
	room.HandleJoin("p1", nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		h.writePump(room, "p1", c, ctx)
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
	time.Sleep(100 * time.Millisecond)
}
