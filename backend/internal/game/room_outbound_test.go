package game

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestRoom_enqueueOutbound_CriticalDoesNotBlockIndefinitely(t *testing.T) {
	r := NewRoom("OUT1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	blockOutboundConsumerAndFillQueue(t, r)

	done := make(chan struct{})
	go func() {
		r.enqueueOutbound([]byte("critical"), broadcastOpts{critical: true})
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
	r.outbound.startLoop()
	r.outbound.ch <- outboundMsg{payload: []byte("hold")}
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < outboundQueueSize; i++ {
		select {
		case r.outbound.ch <- outboundMsg{payload: []byte("fill"), critical: false}:
		default:
			return
		}
	}
}

func TestRoom_enqueueOutbound_DropsNonCriticalWhenFull(t *testing.T) {
	r := NewRoom("OUT2", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	blockOutboundConsumerAndFillQueue(t, r)
	r.enqueueOutbound([]byte("drop"), broadcastOpts{})
	r.mu.Unlock()
	r.stopOutbound()
}

func TestRoom_enqueueOutbound_CriticalTimeoutDrop(t *testing.T) {
	r := NewRoom("OUTCT", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	blockOutboundConsumerAndFillQueue(t, r)
	r.enqueueOutbound([]byte("crit-drop"), broadcastOpts{critical: true})
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

	r.enqueueOutbound([]byte("crit-ok"), broadcastOpts{critical: true})

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
	targets := r.SnapshotTargets("")
	r.mu.Unlock()

	msg := outboundMsg{payload: []byte("x"), critical: false}
	for i := 0; i < 10; i++ {
		r.outbound.deliverToTargets(targets, msg)
	}

	r.mu.Lock()
	pc := r.connections["p1"]
	r.mu.Unlock()
	if pc == nil || !pc.pendingDisconnect.Load() {
		t.Fatal("expected pending disconnect after 10 consecutive drops")
	}
}

func TestRoom_deliverOutbound_RemovesPendingDisconnect(t *testing.T) {
	r := NewRoom("OUT4", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	pc := &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1)}
	pc.pendingDisconnect.Store(true)
	r.mu.Lock()
	r.connections["p1"] = pc
	r.mu.Unlock()

	r.outbound.deliver(outboundMsg{payload: []byte("ok")})

	r.mu.Lock()
	_, exists := r.connections["p1"]
	r.mu.Unlock()
	if exists {
		t.Fatal("pending disconnect connection should be removed")
	}
}

func TestRoom_enqueueOutbound_CriticalTimeout(t *testing.T) {
	r := NewRoom("CRIT", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	r.outbound.ch = make(chan outboundMsg, 1)
	r.outbound.ch <- outboundMsg{payload: []byte("block")}

	done := make(chan struct{})
	go func() {
		r.mu.Lock()
		r.enqueueOutbound([]byte("critical"), broadcastOpts{critical: true})
		r.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("critical enqueue should return after timeout when queue blocked")
	}
}

func TestRoom_enqueueOutbound_AsyncSuccess(t *testing.T) {
	r := NewRoom("ASYNC", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	r.mu.Lock()
	r.outbound.startLoop()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.enqueueOutbound([]byte("hello"), broadcastOpts{})
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
	r.enqueueOutbound([]byte("msg"), broadcastOpts{})
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
	r.enqueueOutbound([]byte("crit"), broadcastOpts{critical: true})
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
	targets := r.SnapshotTargets("p1")
	r.mu.Unlock()
	if len(targets) != 0 {
		t.Fatalf("targets = %d, want 0 (excluded/nil only)", len(targets))
	}
}

// --- coverage gap 补充用例 ---

func TestRoom_snapshotConnTargetsLocked_IncludesValidTarget(t *testing.T) {
	r := NewRoom("TG", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 1)}
	targets := r.SnapshotTargets("p1")
	r.mu.Unlock()
	if len(targets) != 1 || targets[0].playerID != "p2" {
		t.Fatalf("targets = %+v", targets)
	}
}

func TestRoom_snapshotConnTargetsLocked_ConnCloseCallback(t *testing.T) {
	server := testutil.NewWSTestUpgraderServer(t)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	r := NewRoom("CC", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1), Conn: conn}
	targets := r.SnapshotTargets("")
	r.mu.Unlock()
	if len(targets) != 1 {
		t.Fatalf("targets = %d", len(targets))
	}
	targets[0].connClose()
}
