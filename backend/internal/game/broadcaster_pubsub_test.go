package game

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/config"
)

func TestPubSubBroadcaster_PublishSubscribe(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	b := NewPubSubBroadcaster(rdb)
	defer b.Close()

	received := make(chan BroadcastMessage, 1)
	unsub, err := b.Subscribe("ROOM1", func(msg BroadcastMessage) {
		received <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	msg := BroadcastMessage{Payload: []byte("hello"), Critical: true}
	if err := b.Publish(context.Background(), "ROOM1", msg); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-received:
		if string(got.Payload) != "hello" {
			t.Fatalf("payload = %q, want hello", string(got.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pubsub message")
	}
}

func TestPubSubBroadcaster_PublishNotConnected(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	b := NewPubSubBroadcaster(rdb)
	b.connected.Store(false)

	err = b.Publish(context.Background(), "ROOM1", BroadcastMessage{Payload: []byte("x")})
	if err == nil {
		t.Fatal("expected error when broadcaster disconnected")
	}
}

func TestChannelName(t *testing.T) {
	if got := channelName("ABCDE"); got != "room:ABCDE:broadcast" {
		t.Fatalf("channelName = %q", got)
	}
}

func TestDefaultInstanceID_Env(t *testing.T) {
	t.Setenv("INSTANCE_ID", "test-instance")
	if got := defaultInstanceID(); got != "test-instance" {
		t.Fatalf("defaultInstanceID = %q", got)
	}
}

func TestHub_PlayerCountAndPhaseCounts(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	addConnectedPlayer(room, "p1")
	addConnectedPlayer(room, "p2")

	if h.PlayerCount() != 2 {
		t.Fatalf("PlayerCount = %d, want 2", h.PlayerCount())
	}
	counts := h.PhaseCounts()
	if counts["waiting"] != 1 {
		t.Fatalf("phase counts = %+v", counts)
	}
}

func TestPubSubBroadcaster_Close(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	b := NewPubSubBroadcaster(rdb)
	unsub, err := b.Subscribe("ROOM1", func(BroadcastMessage) {})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if b.connected.Load() {
		t.Fatal("expected disconnected after Close")
	}
	if err := b.Publish(context.Background(), "ROOM1", BroadcastMessage{Payload: []byte("x")}); err == nil {
		t.Fatal("expected publish error after Close")
	}
}

func TestHub_CloseAllRooms(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	_, _ = h.CreateRoom(context.Background())
	_, _ = h.CreateRoom(context.Background())

	h.CloseAllRooms()
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d after CloseAllRooms", h.RoomCount())
	}
}
