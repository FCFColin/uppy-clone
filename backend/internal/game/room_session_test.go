package game

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestRoom_MaybeStartReadSpan(t *testing.T) {
	room := NewRoom("SPAN1", nil, nil, config.DefaultTimeoutConfig(), 4)
	sess := &WSSession{room: room}

	t.Run("PingNoSpan", func(t *testing.T) {
		var counter uint64
		if span := sess.maybeStartReadSpan(context.Background(), "p1", protocol.MsgPing, &counter); span != nil {
			t.Fatal("ping should not create span")
		}
	})

	t.Run("FirstTapNoSpan", func(t *testing.T) {
		var counter uint64
		if span := sess.maybeStartReadSpan(context.Background(), "p1", protocol.MsgTap, &counter); span != nil {
			t.Fatal("first tap should not create span")
		}
	})

	t.Run("HundredthTapSpan", func(t *testing.T) {
		counter := uint64(99)
		if span := sess.maybeStartReadSpan(context.Background(), "p1", protocol.MsgTap, &counter); span == nil {
			t.Fatal("100th tap should create span")
		}
	})

	t.Run("SetNicknameSpan", func(t *testing.T) {
		counter := uint64(99)
		span := sess.maybeStartReadSpan(context.Background(), "p1", protocol.MsgSetNickname, &counter)
		if span == nil {
			t.Fatal("set_nickname should create span")
		}
		span.End()
	})

	t.Run("RestartVoteSpan", func(t *testing.T) {
		span := sess.maybeStartReadSpan(context.Background(), "p1", protocol.MsgRestartVote, new(uint64))
		if span == nil {
			t.Fatal("restart_vote should create span")
		} else {
			span.End()
		}
	})

	t.Run("UnknownMsgSpan", func(t *testing.T) {
		span := sess.maybeStartReadSpan(context.Background(), "p1", 0xFF, new(uint64))
		if span == nil {
			t.Fatal("unknown message should create span")
		} else {
			span.End()
		}
	})
}

func TestRoom_WritePump_WriteMessageError(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	timeouts.WSPingInterval = time.Hour
	hub := NewHub(nil, nil, timeouts, 10, 8)
	room := NewRoom("PUMP3", hub, nil, timeouts, 4)
	sess := &WSSession{room: room}
	if err := room.HandleJoin("p1", nil); err != nil {
		t.Fatal(err)
	}
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
		go sess.writePump("p1", c, ctx)
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

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
}

func TestRoom_RunSession_HandleJoinFailure(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := NewHub(nil, nil, timeouts, 10, 1)
	code, err := hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room := hub.getRoom(code)
	if err := room.HandleJoin("existing", nil); err != nil {
		t.Fatalf("HandleJoin: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = room.RunSession(r.Context(), "user2", c)
	}))
	defer server.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
}

func TestRoom_ReadPump_HandleMessageErrorWithSpan(t *testing.T) {
	room := NewRoom("RSPN", NewHub(nil, nil, config.DefaultTimeoutConfig(), 10, 8), nil, config.DefaultTimeoutConfig(), 4)
	sess := &WSSession{room: room}

	if err := room.HandleJoin("p1", nil); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		sess.readPump("p1", c, ctx, cancel)
	}))
	defer server.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{protocol.MsgSetNickname, 'x'}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
}
