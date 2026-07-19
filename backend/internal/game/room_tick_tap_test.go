package game

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestRoom_HandleTap(t *testing.T) {
	cases := []struct {
		name          string
		phase         domain.GamePhase
		cooldownEnd   int64
		payload       []byte
		wantMsg       byte
		wantScoreIncr bool
	}{
		{
			name:          "AcceptsValidTap",
			phase:         domain.PhasePlaying,
			payload:       encodeTapTestPayload(0.5, 0.5),
			wantMsg:       protocol.MsgTapAccepted,
			wantScoreIncr: true,
		},
		{
			name:    "RejectsWrongPhase",
			phase:   domain.PhaseWaiting,
			payload: encodeTapTestPayload(0.5, 0.5),
			wantMsg: protocol.MsgTapRejected,
		},
		{
			name:        "RejectsCooldown",
			phase:       domain.PhasePlaying,
			cooldownEnd: time.Now().UnixMilli() + 5000,
			payload:     encodeTapTestPayload(0.5, 0.5),
			wantMsg:     protocol.MsgTapRejected,
		},
		{
			name:    "RejectsInvalidPayload",
			phase:   domain.PhasePlaying,
			payload: []byte{0x01},
			wantMsg: protocol.MsgTapRejected,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			timeouts := config.DefaultTimeoutConfig()
			r := NewRoom("TEST1", nil, nil, timeouts, 0)
			r.syncOutbound = true
			r.state.Phase = c.phase
			r.state.Balloon.X = 0.5
			r.state.Balloon.Y = 0.5

			player := &domain.PlayerState{ID: "p1", PlayerIndex: 0, CooldownEndTime: c.cooldownEnd}
			ch := make(chan []byte, 64)
			r.mu.Lock()
			r.state.Players["p1"] = player
			r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
			r.mu.Unlock()

			beforeScore := r.state.Balloon.Score
			r.handleTap(player, "p1", c.payload)

			if c.wantScoreIncr && r.state.Balloon.Score != beforeScore+1 {
				t.Fatalf("score = %d, want %d", r.state.Balloon.Score, beforeScore+1)
			}
			if c.wantScoreIncr && player.TapsCount != 1 {
				t.Fatalf("TapsCount = %d, want 1", player.TapsCount)
			}
			select {
			case msg := <-ch:
				if len(msg) == 0 || msg[0] != c.wantMsg {
					t.Fatalf("expected msg 0x%02x, got %v", c.wantMsg, msg)
				}
			default:
				t.Fatal("expected message on player channel")
			}
		})
	}
}

func TestRoom_ApplyTapPhysics(t *testing.T) {
	r := &Room{state: NewGameState("TEST", 42, testRNG())}
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

func TestRoom_decodeTapPayload_DecodeFailure(t *testing.T) {
	r := &Room{state: NewGameState("T", 42, testRNG()), rng: testRNG()}
	_, _, ok := r.decodeTapPayload([]byte{1, 2, 3})
	if ok {
		t.Fatal("short payload should fail decode")
	}
}
