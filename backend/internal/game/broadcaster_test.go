package game

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
)

// mockBroadcaster 是基于内存的 Broadcaster 实现，用于单元测试（无需真实 Redis）。
type mockBroadcaster struct {
	mu        sync.Mutex
	handlers  map[string]func(BroadcastMessage)
	published []BroadcastMessage
	closed    bool
}

func newMockBroadcaster() *mockBroadcaster {
	return &mockBroadcaster{handlers: make(map[string]func(BroadcastMessage))}
}

func (m *mockBroadcaster) Publish(_ context.Context, roomCode string, msg BroadcastMessage) error {
	m.mu.Lock()
	m.published = append(m.published, msg)
	handler := m.handlers[roomCode]
	m.mu.Unlock()
	if handler != nil {
		handler(msg)
	}
	return nil
}

func (m *mockBroadcaster) Subscribe(roomCode string, handler func(BroadcastMessage)) (func(), error) {
	m.mu.Lock()
	m.handlers[roomCode] = handler
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.handlers, roomCode)
		m.mu.Unlock()
	}, nil
}

func (m *mockBroadcaster) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.handlers = make(map[string]func(BroadcastMessage))
	return nil
}

// ─── Broadcaster Publish/Subscribe ───────────────────────────────────

func TestMockBroadcaster_PublishSubscribe(t *testing.T) {
	b := newMockBroadcaster()
	defer b.Close()

	received := make(chan BroadcastMessage, 1)
	unsub, err := b.Subscribe("ROOM1", func(msg BroadcastMessage) {
		received <- msg
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer unsub()

	msg := BroadcastMessage{Payload: []byte("hello")}
	if err := b.Publish(context.Background(), "ROOM1", msg); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case got := <-received:
		if string(got.Payload) != "hello" {
			t.Fatalf("expected payload 'hello', got %q", string(got.Payload))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestMockBroadcaster_Unsubscribe(t *testing.T) {
	b := newMockBroadcaster()
	defer b.Close()

	received := make(chan BroadcastMessage, 1)
	unsub, _ := b.Subscribe("ROOM1", func(msg BroadcastMessage) {
		received <- msg
	})
	unsub()

	_ = b.Publish(context.Background(), "ROOM1", BroadcastMessage{Payload: []byte("nope")})

	select {
	case <-received:
		t.Fatal("should not receive after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// ─── excludePlayer ───────────────────────────────────────────────────

func TestRoom_BroadcastLocal_ExcludePlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	ch1 := make(chan []byte, 64)
	ch2 := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch1}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: ch2}
	r.mu.Unlock()

	msg := []byte{0x01, 0x02, 0x03}
	r.broadcastLocal(msg, "p1")

	select {
	case <-ch1:
		t.Fatal("p1 should NOT receive (excluded)")
	default:
	}
	select {
	case <-ch2:
		// expected
	default:
		t.Fatal("p2 should receive")
	}
}

func TestRoom_Broadcast_PublishesExcludePlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	b := newMockBroadcaster()
	defer b.Close()

	h := NewHub(nil, nil, timeouts, 0, 0, b)
	r := NewRoom("ROOM1", h, nil, timeouts, 0)

	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	r.broadcast([]byte{0xFF}, "p1")

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(b.published))
	}
	if b.published[0].ExcludePlayer != "p1" {
		t.Fatalf("expected ExcludePlayer 'p1', got %q", b.published[0].ExcludePlayer)
	}
}

// ─── excludeInstance prevents loops ──────────────────────────────────

func TestHub_HandleRemoteBroadcast_ExcludeInstance(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	b := newMockBroadcaster()
	defer b.Close()

	h := NewHub(nil, nil, timeouts, 0, 0, b)
	h.instanceID = "instance-A"

	room := NewRoom("ROOM1", h, nil, timeouts, 0)
	h.mu.Lock()
	h.rooms["ROOM1"] = room
	h.mu.Unlock()

	ch := make(chan []byte, 64)
	room.mu.Lock()
	room.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	room.mu.Unlock()

	// 同实例发出的消息 → 应跳过
	h.handleRemoteBroadcast("ROOM1", BroadcastMessage{
		ExcludeInstance: "instance-A",
		Payload:         []byte("skip"),
	})
	select {
	case <-ch:
		t.Fatal("should not receive message from same instance")
	default:
	}

	// 不同实例发出的消息 → 应投递
	h.handleRemoteBroadcast("ROOM1", BroadcastMessage{
		ExcludeInstance: "instance-B",
		Payload:         []byte("deliver"),
	})
	select {
	case got := <-ch:
		if string(got) != "deliver" {
			t.Fatalf("expected 'deliver', got %q", string(got))
		}
	default:
		t.Fatal("should receive message from different instance")
	}
}

func TestHub_HandleRemoteBroadcast_RoomNotFound(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	b := newMockBroadcaster()
	defer b.Close()

	h := NewHub(nil, nil, timeouts, 0, 0, b)
	h.instanceID = "instance-A"

	// 房间不存在 → 不应 panic
	h.handleRemoteBroadcast("NONEXISTENT", BroadcastMessage{
		ExcludeInstance: "instance-B",
		Payload:         []byte("data"),
	})
}

// ─── nil broadcaster (single-instance mode) ──────────────────────────

func TestRoom_NilBroadcaster_NoPanic(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	// r.broadcaster is nil (hub is nil)

	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	// Should not panic
	r.broadcast([]byte{0x01}, "")

	select {
	case <-ch:
		// expected — local delivery still works
	default:
		t.Fatal("p1 should receive message even with nil broadcaster")
	}
}

func TestRoom_NilBroadcaster_CriticalNoPanic(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	// Should not panic
	r.broadcastCritical([]byte{0x02})

	select {
	case <-ch:
		// expected
	default:
		t.Fatal("p1 should receive critical message even with nil broadcaster")
	}
}
