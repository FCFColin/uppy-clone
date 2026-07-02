package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
)

type publishErrBroadcaster struct{}

func (e *publishErrBroadcaster) Publish(_ context.Context, _ string, _ BroadcastMessage) error {
	return errors.New("publish failed")
}

func (e *publishErrBroadcaster) Subscribe(_ string, _ func(BroadcastMessage)) (func(), error) {
	return func() {}, nil
}

func (e *publishErrBroadcaster) Close() error { return nil }

type subscribeErrBroadcaster struct {
	mockBroadcaster
}

func (s *subscribeErrBroadcaster) Subscribe(_ string, _ func(BroadcastMessage)) (func(), error) {
	return nil, errors.New("subscribe failed")
}

func (s *subscribeErrBroadcaster) Publish(ctx context.Context, roomCode string, msg BroadcastMessage) error {
	return s.mockBroadcaster.Publish(ctx, roomCode, msg)
}

func (s *subscribeErrBroadcaster) Close() error { return s.mockBroadcaster.Close() }

func TestRoom_enqueueOutbound_CriticalDoesNotBlockIndefinitely(t *testing.T) {
	r := NewRoom("OUT1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	blockOutboundConsumerAndFillQueue(t, r)

	done := make(chan struct{})
	go func() {
		r.enqueueOutbound([]byte("critical"), "", true, false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		r.mu.Unlock()
		r.stopOutbound()
		t.Fatal("critical enqueue blocked longer than timeout")
	}
	r.mu.Unlock()
	r.stopOutbound()
}

func blockOutboundConsumerAndFillQueue(t *testing.T, r *Room) {
	t.Helper()
	r.mu.Lock()
	r.startOutboundLoop()
	r.outboundCh <- outboundMsg{payload: []byte("hold"), skipRedis: true}
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < outboundQueueSize; i++ {
		select {
		case r.outboundCh <- outboundMsg{payload: []byte("fill"), critical: false}:
		default:
			return
		}
	}
}

func TestRoom_enqueueOutbound_DropsNonCriticalWhenFull(t *testing.T) {
	r := NewRoom("OUT2", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	blockOutboundConsumerAndFillQueue(t, r)
	r.enqueueOutbound([]byte("drop"), "", false, false)
	r.mu.Unlock()
	r.stopOutbound()
}

func TestRoom_enqueueOutbound_CriticalTimeoutDrop(t *testing.T) {
	r := NewRoom("OUTCT", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	blockOutboundConsumerAndFillQueue(t, r)
	r.enqueueOutbound([]byte("crit-drop"), "", true, false)
	r.mu.Unlock()
	r.stopOutbound()
}

func TestRoom_enqueueOutbound_CriticalRetrySuccess(t *testing.T) {
	r := NewRoom("OUTRS", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	blockOutboundConsumerAndFillQueue(t, r)

	done := make(chan struct{})
	go func() {
		time.Sleep(15 * time.Millisecond)
		r.mu.Unlock()
	}()

	r.enqueueOutbound([]byte("crit-ok"), "", true, false)

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
	}
	r.stopOutbound()
}

func TestRoom_deliverToTargets_SlowClientDisconnect(t *testing.T) {
	r := NewRoom("OUT3", nil, nil, config.DefaultTimeoutConfig(), 0)
	ch := make(chan []byte)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	targets := r.snapshotConnTargetsLocked("")
	r.mu.Unlock()

	msg := outboundMsg{payload: []byte("x"), critical: false}
	for i := 0; i < 10; i++ {
		r.deliverToTargets(targets, msg)
	}

	r.mu.Lock()
	pc := r.connections["p1"]
	r.mu.Unlock()
	if pc == nil || !pc.pendingDisconnect {
		t.Fatal("expected pending disconnect after 10 consecutive drops")
	}
}

func TestRoom_deliverOutbound_RemovesPendingDisconnect(t *testing.T) {
	r := NewRoom("OUT4", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1), pendingDisconnect: true}
	r.mu.Unlock()

	r.deliverOutbound(outboundMsg{payload: []byte("ok"), skipRedis: true})

	r.mu.Lock()
	_, exists := r.connections["p1"]
	r.mu.Unlock()
	if exists {
		t.Fatal("pending disconnect connection should be removed")
	}
}

func TestRoom_publishBroadcastAsync_PublishError(t *testing.T) {
	r := NewRoom("OUT5", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.broadcaster = &publishErrBroadcaster{}
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	r.broadcast([]byte{0x01}, "")
	r.mu.Unlock()
}

func TestRoom_enqueueOutbound_SyncPath(t *testing.T) {
	r := NewRoom("SYNC", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.enqueueOutbound([]byte("sync-msg"), "", false, true)
	r.mu.Unlock()
}

func TestRoom_enqueueOutbound_CriticalTimeout(t *testing.T) {
	r := NewRoom("CRIT", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	r.outboundCh = make(chan outboundMsg, 1)
	r.outboundCh <- outboundMsg{payload: []byte("block")}

	done := make(chan struct{})
	go func() {
		r.mu.Lock()
		r.enqueueOutbound([]byte("critical"), "", true, false)
		r.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("critical enqueue should return after timeout when queue blocked")
	}
}

func TestRoom_stopOutbound(t *testing.T) {
	r := NewRoom("STOP", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	r.startOutboundLoop()
	r.stopOutbound()
}

func TestRoom_enqueueOutbound_AsyncSuccess(t *testing.T) {
	r := NewRoom("ASYNC", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	r.mu.Lock()
	r.startOutboundLoop()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.enqueueOutbound([]byte("hello"), "", false, true)
	r.mu.Unlock()

	select {
	case msg := <-r.connections["p1"].Send:
		if string(msg) != "hello" {
			t.Fatalf("msg = %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected async delivery")
	}
	r.stopOutbound()
}

func TestRoom_enqueueOutbound_NonCriticalNotFull(t *testing.T) {
	r := NewRoom("NF", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.enqueueOutbound([]byte("msg"), "", false, true)
	r.mu.Unlock()
	select {
	case msg := <-r.connections["p1"].Send:
		if string(msg) != "msg" {
			t.Fatalf("msg = %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected delivery")
	}
	r.stopOutbound()
}

func TestRoom_enqueueOutbound_CriticalNotFull(t *testing.T) {
	r := NewRoom("CF", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.enqueueOutbound([]byte("crit"), "", true, true)
	r.mu.Unlock()
	select {
	case <-r.connections["p1"].Send:
	case <-time.After(time.Second):
		t.Fatal("expected critical delivery")
	}
	r.stopOutbound()
}

func TestRoom_snapshotConnTargetsLocked_SkipsNilAndExcluded(t *testing.T) {
	r := NewRoom("SN", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: nil}
	r.connections["p3"] = nil
	targets := r.snapshotConnTargetsLocked("p1")
	r.mu.Unlock()
	if len(targets) != 0 {
		t.Fatalf("targets = %d, want 0 (excluded/nil only)", len(targets))
	}
}

func TestRoom_stopOutbound_NoLoopStarted(t *testing.T) {
	r := NewRoom("NS", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.stopOutbound()
}
