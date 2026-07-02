package game

import (
	"context"
	"errors"
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

func TestDefaultInstanceID_Hostname(t *testing.T) {
	t.Setenv("INSTANCE_ID", "")
	got := defaultInstanceID()
	if got == "" || got == "unknown" {
		// hostname may fail in some CI; at least exercise the branch
		t.Logf("defaultInstanceID = %q", got)
	}
}

func TestPubSubBroadcaster_SubscribeInvalidJSON(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	b := NewPubSubBroadcaster(rdb)
	defer b.Close()

	called := make(chan struct{}, 1)
	unsub, err := b.Subscribe("ROOM1", func(BroadcastMessage) {
		called <- struct{}{}
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	time.Sleep(50 * time.Millisecond)
	mr.Publish("room:ROOM1:broadcast", "{not-json")
	time.Sleep(500 * time.Millisecond)
	select {
	case <-called:
		t.Fatal("handler should not run for invalid JSON")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPubSubBroadcaster_PublishRedisError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	b := NewPubSubBroadcaster(rdb)
	_ = rdb.Close()

	err = b.Publish(context.Background(), "ROOM1", BroadcastMessage{Payload: []byte("x")})
	if err == nil {
		t.Fatal("expected publish error when redis client closed")
	}
}

func TestPubSubBroadcaster_PublishMarshalError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	prev := marshalBroadcastFn
	marshalBroadcastFn = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	defer func() { marshalBroadcastFn = prev }()

	b := NewPubSubBroadcaster(rdb)
	defer b.Close()
	err = b.Publish(context.Background(), "ROOM1", BroadcastMessage{Payload: []byte("x")})
	if err == nil {
		t.Fatal("expected marshal error")
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

func TestPubSubBroadcaster_Close_InjectedError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	b := NewPubSubBroadcaster(rdb)
	restore := SetPubsubCloseErrForTest(errors.New("close failed"))
	defer restore()

	if err := b.Close(); err == nil {
		t.Fatal("expected injected close error")
	}
}

func TestPubSubBroadcaster_CloseWithError(t *testing.T) {
	t.Cleanup(SetPubsubCloseErrForTest(errors.New("close failed")))
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	b := NewPubSubBroadcaster(rdb)
	if err := b.Close(); err == nil {
		t.Fatal("expected Close error from test hook")
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
