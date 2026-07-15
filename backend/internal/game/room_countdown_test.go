package game

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestRemainingCountdownMs(t *testing.T) {
	start := time.Now().UnixMilli() - 1000
	if got := remainingCountdownMs(start); got < 100 {
		t.Fatalf("remaining = %d", got)
	}
	startFar := time.Now().UnixMilli() - countdownDurationMs()
	if got := remainingCountdownMs(startFar); got != 100 {
		t.Fatalf("minimum remaining = %d, want 100", got)
	}
}

func TestScheduleCountdownFromNow(t *testing.T) {
	r := NewRoom("CD", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.scheduleCountdownFromNow()
	if r.endGameTimer == nil {
		t.Fatal("expected countdown timer")
	}
}

func TestResumeCountdownForReconnect(_ *testing.T) {
	r := NewRoom("RC", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseCountdown
	r.countdownStart = time.Now().UnixMilli()
	addConnectedPlayer(r, "p1")
	r.resumeCountdownForReconnect("p1")
}

func TestCloseExistingConnection(t *testing.T) {
	r := NewRoom("CC", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "p1", Disconnected: true}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1)}
	r.closeExistingConnection("p1", player)
	if _, ok := r.connections["p1"]; !ok {
		t.Fatal("disconnected player should not remove connection")
	}
}

func TestCloseExistingConnection_DisconnectedPlayer(_ *testing.T) {
	r := NewRoom("CC2", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Disconnected: true}
	r.closeExistingConnection("p1", player)
}

func TestSetEndGameAlarm_Countdown(_ *testing.T) {
	r := NewRoom("EA", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseCountdown
	r.setEndGameAlarm(time.Now())
	time.Sleep(10 * time.Millisecond)
}

func TestReconnectPlayer_Countdown(t *testing.T) {
	r := NewRoom("RP", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseCountdown
	r.countdownStart = time.Now().UnixMilli()
	player := &domain.PlayerState{ID: "p1", Nickname: "p1", Disconnected: true}
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.reconnectPlayer("p1", player)
	if player.Disconnected {
		t.Fatal("player should be reconnected")
	}
}
