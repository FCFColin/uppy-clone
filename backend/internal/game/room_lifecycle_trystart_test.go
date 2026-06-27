package game

import (
	"log/slog"
	"os"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
)

func TestTryStartWhenAllReady_WrongPhase(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhasePlaying
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhasePlaying {
		t.Error("tryStartWhenAllReady should not change phase when not waiting")
	}
}

func TestTryStartWhenAllReady_NotAllReady(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:       NewGameState("TEST"),
		connections: make(map[string]*PlayerConn),
		usedNames:   make(map[string]bool),
		logger:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhaseWaiting
	room.state.Players["p1"] = &domain.PlayerState{
		Nickname:          "Alice",
		PlayerIndex:       0,
		NicknameConfirmed: false,
	}
	room.connections["p1"] = &PlayerConn{}
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhaseWaiting {
		t.Errorf("Phase should remain waiting, got %v", room.state.Phase)
	}
}

func TestTryStartWhenAllReady_NoConnections(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhaseWaiting
	room.state.Players["p1"] = &domain.PlayerState{
		Nickname:          "Alice",
		PlayerIndex:       0,
		NicknameConfirmed: true,
	}
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhaseWaiting {
		t.Errorf("Phase should remain waiting when no connections, got %v", room.state.Phase)
	}
}

func TestTryStartWhenAllReady_EmptyPlayers(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhaseWaiting
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhaseWaiting {
		t.Errorf("Phase should remain waiting with no players, got %v", room.state.Phase)
	}
}
