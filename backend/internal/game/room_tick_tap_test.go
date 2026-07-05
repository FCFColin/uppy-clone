package game

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestRoom_HandleTap_AcceptsValidTap(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhasePlaying
	r.state.Balloon.X = 0.5
	r.state.Balloon.Y = 0.5

	player := &domain.PlayerState{ID: "p1", PlayerIndex: 0, CooldownEndTime: 0}
	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	beforeScore := r.state.Balloon.Score
	r.handleTap(player, "p1", encodeTapTestPayload(0.5, 0.5))

	if r.state.Balloon.Score != beforeScore+1 {
		t.Fatalf("score = %d, want %d", r.state.Balloon.Score, beforeScore+1)
	}
	if player.TapsCount != 1 {
		t.Fatalf("TapsCount = %d, want 1", player.TapsCount)
	}
	select {
	case msg := <-ch:
		if len(msg) == 0 || msg[0] != protocol.MsgTapAccepted {
			t.Fatalf("expected tap accepted message, got %v", msg)
		}
	default:
		t.Fatal("expected tap accepted message on player channel")
	}
}

func TestRoom_HandleTap_RejectsWrongPhase(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhaseWaiting

	player := &domain.PlayerState{ID: "p1", PlayerIndex: 0}
	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	r.handleTap(player, "p1", encodeTapTestPayload(0.5, 0.5))

	select {
	case msg := <-ch:
		if len(msg) == 0 || msg[0] != protocol.MsgTapRejected {
			t.Fatalf("expected tap rejected, got %v", msg)
		}
	default:
		t.Fatal("expected tap rejected message")
	}
}

func TestRoom_HandleTap_RejectsCooldown(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhasePlaying

	now := time.Now().UnixMilli()
	player := &domain.PlayerState{ID: "p1", PlayerIndex: 0, CooldownEndTime: now + 5000}
	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	r.handleTap(player, "p1", encodeTapTestPayload(0.5, 0.5))

	select {
	case msg := <-ch:
		if len(msg) == 0 || msg[0] != protocol.MsgTapRejected {
			t.Fatalf("expected tap rejected on cooldown, got %v", msg)
		}
	default:
		t.Fatal("expected tap rejected message")
	}
}

func TestRoom_HandleTap_RejectsInvalidPayload(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhasePlaying

	player := &domain.PlayerState{ID: "p1", PlayerIndex: 0}
	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	r.handleTap(player, "p1", []byte{0x01})

	select {
	case msg := <-ch:
		if len(msg) == 0 || msg[0] != protocol.MsgTapRejected {
			t.Fatalf("expected tap rejected for bad payload, got %v", msg)
		}
	default:
		t.Fatal("expected tap rejected message")
	}
}

func TestRoom_ApplyTapPhysics(t *testing.T) {
	r := &Room{state: NewGameState("TEST", testRNG())}
	r.state.Balloon.X = 0.5
	r.state.Balloon.Y = 0.5
	if !r.applyTapPhysics(0.5, 0.5) {
		t.Fatal("applyTapPhysics should succeed for valid coordinates")
	}
}

func TestRoom_BroadcastTapResult(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhasePlaying

	player := &domain.PlayerState{ID: "p1", PlayerIndex: 1}
	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	r.broadcastTapResult(player, 500)

	select {
	case msg := <-ch:
		if len(msg) == 0 || msg[0] != protocol.MsgTapAccepted {
			t.Fatalf("expected tap accepted broadcast, got %v", msg)
		}
	default:
		t.Fatal("expected broadcast message")
	}
}
