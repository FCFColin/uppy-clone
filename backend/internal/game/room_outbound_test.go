package game

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestRoom_OutboundQueue_CriticalPriority(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true

	ch := make(chan []byte, 4)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.broadcastCritical([]byte{0xAA})
	r.mu.Unlock()

	select {
	case got := <-ch:
		if len(got) != 1 || got[0] != 0xAA {
			t.Fatalf("unexpected payload: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("critical message not delivered")
	}
}

func TestRoom_AsyncPersist_FlushesOnClose(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	store := newMockRoomRepository()
	r := NewRoom("PERSIST", nil, store, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = "waiting"
	r.mu.Unlock()

	r.saveState()
	time.Sleep(200 * time.Millisecond)

	r.Close()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.lobbyStates) == 0 {
		t.Fatal("expected persisted state after close")
	}
}

func TestRoom_TickAndMessageNotBlockedByOutbound(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("CONC", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "p1"}
	r.connections["p1"] = &PlayerConn{
		PlayerID: "p1",
		Send:     make(chan []byte, 256),
	}
	r.mu.Unlock()

	start := time.Now()
	r.mu.Lock()
	r.tickOnce()
	r.mu.Unlock()
	if d := time.Since(start); d > 10*time.Millisecond {
		t.Fatalf("tickOnce held lock too long: %v", d)
	}

	done := make(chan time.Duration, 16)
	for i := 0; i < 16; i++ {
		go func() {
			start := time.Now()
			_ = r.HandleMessage("p1", protocol.MsgPing, nil)
			done <- time.Since(start)
		}()
	}

	for i := 0; i < 16; i++ {
		select {
		case d := <-done:
			if d > 20*time.Millisecond {
				t.Fatalf("HandleMessage blocked too long: %v", d)
			}
		case <-time.After(time.Second):
			t.Fatal("HandleMessage goroutine timed out")
		}
	}

	r.Close()
}
