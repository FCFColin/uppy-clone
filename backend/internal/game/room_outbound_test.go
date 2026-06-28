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

func TestRoom_enqueueOutbound_CriticalDoesNotBlockIndefinitely(t *testing.T) {
	r := NewRoom("OUT1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	r.startOutboundLoop()
	r.outboundCh = make(chan outboundMsg, 1)
	r.outboundCh <- outboundMsg{payload: []byte("fill"), critical: false}

	done := make(chan struct{})
	go func() {
		r.mu.Lock()
		r.enqueueOutbound([]byte("critical"), "", true, false)
		r.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("critical enqueue blocked longer than timeout")
	}
}

func TestRoom_enqueueOutbound_DropsNonCriticalWhenFull(t *testing.T) {
	r := NewRoom("OUT2", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	r.outboundCh = make(chan outboundMsg, 1)
	r.outboundCh <- outboundMsg{payload: []byte("fill")}

	done := make(chan struct{})
	go func() {
		r.mu.Lock()
		r.enqueueOutbound([]byte("drop"), "", false, false)
		r.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("non-critical enqueue should not block when queue full")
	}
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
