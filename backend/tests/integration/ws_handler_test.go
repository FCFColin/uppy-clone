//go:build integration

package integration

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
	"github.com/uppy-clone/backend/internal/handler"
)

func authWSMiddleware(userID, nickname string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, nickname)
		next(w, r.WithContext(ctx))
	}
}

func wsDial(server *httptest.Server, code, origin string) (*websocket.Conn, *http.Response, error) {
	url := "ws" + server.URL[4:] + "/lobby/" + code + "/ws"
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	conn, resp, err := dialer.Dial(url, hdr)
	return conn, resp, err
}

func TestWSHandler_ConnectAndDisconnect(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 100, 50, nil)
	lobbyHandler := handler.NewLobbyHandler(hub, []string{"http://localhost"})

	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", authWSMiddleware("user1", "nick1", lobbyHandler.WebSocket))
	server := httptest.NewServer(r)
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, resp, err := wsDial(server, code, "http://localhost")
	if err != nil {
		t.Fatalf("wsDial: %v (resp=%+v)", err, resp)
	}
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if len(msg) == 0 {
		t.Fatal("expected non-empty snapshot message")
	}
}

func TestWSHandler_InvalidRoomRejected(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 100, 50, nil)
	lobbyHandler := handler.NewLobbyHandler(hub, []string{"http://localhost"})

	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", authWSMiddleware("user1", "nick1", lobbyHandler.WebSocket))
	server := httptest.NewServer(r)
	defer server.Close()

	conn, resp, err := wsDial(server, "NOPE1", "http://localhost")
	if err == nil && conn != nil {
		conn.Close()
		t.Fatal("expected connection to be rejected")
	}
	if resp == nil {
		t.Fatal("expected HTTP response")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWSHandler_UnauthenticatedRejected(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 100, 50, nil)
	lobbyHandler := handler.NewLobbyHandler(hub, []string{"http://localhost"})

	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", lobbyHandler.WebSocket)
	server := httptest.NewServer(r)
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, resp, err := wsDial(server, code, "http://localhost")
	if err == nil && conn != nil {
		conn.Close()
		t.Fatal("expected unauthorized connection to be rejected")
	}
	if resp == nil {
		t.Fatal("expected HTTP response")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestWSHandler_ForbiddenOriginRejected(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 100, 50, nil)
	lobbyHandler := handler.NewLobbyHandler(hub, []string{"http://localhost"})

	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", authWSMiddleware("user1", "nick1", lobbyHandler.WebSocket))
	server := httptest.NewServer(r)
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	conn, resp, err := wsDial(server, code, "http://evil.example.com")
	if err == nil && conn != nil {
		conn.Close()
		t.Fatal("expected connection with forbidden origin to be rejected")
	}
	if resp == nil {
		t.Fatal("expected HTTP response")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestWSHandler_ConnectionLimit(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 2, 50, nil)
	lobbyHandler := handler.NewLobbyHandler(hub, []string{"http://localhost"})

	r := chi.NewRouter()
	r.Get("/lobby/{code}/ws", func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-Test-User-ID")
		if userID == "" {
			userID = "default-user"
		}
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, "nick")
		lobbyHandler.WebSocket(w, r.WithContext(ctx))
	})
	server := httptest.NewServer(r)
	defer server.Close()

	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

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

	hdr3 := http.Header{}
	hdr3.Set("Origin", "http://localhost")
	hdr3.Set("X-Test-User-ID", "user2")
	_, resp3, err := dialer.Dial("ws"+server.URL[4:]+"/lobby/"+code+"/ws", hdr3)
	if err == nil {
		t.Fatal("expected dial 3 to fail (connection limit)")
	}
	if resp3 == nil {
		t.Fatal("expected HTTP response")
	}
	if resp3.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp3.StatusCode)
	}
}
