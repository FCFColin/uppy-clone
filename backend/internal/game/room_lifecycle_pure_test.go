package game

import (
	"log/slog"
	"os"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
)

func TestAllConnectedPlayersReady(t *testing.T) {
	t.Parallel()

	t.Run("empty connections returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false with no connections")
		}
	})

	t.Run("all ready returns true", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: true}
		room.state.Players["p2"] = &domain.PlayerState{Nickname: "Player2", PlayerIndex: 1, NicknameConfirmed: true}
		room.connections["p1"] = &PlayerConn{}
		room.connections["p2"] = &PlayerConn{}

		if !room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return true when all connected players are ready")
		}
	})

	t.Run("player not found returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.connections["ghost"] = &PlayerConn{}

		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false for unknown player")
		}
	})

	t.Run("disconnected player returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: true, Disconnected: true}
		room.connections["p1"] = &PlayerConn{}

		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false for disconnected player")
		}
	})

	t.Run("player not confirmed returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: false}
		room.connections["p1"] = &PlayerConn{}

		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false when nickname not confirmed")
		}
	})
}

func TestNormalizePhaseForNicknameGate(t *testing.T) {
	t.Parallel()

	t.Run("waiting phase unchanged", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
			logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		room.state.Phase = domain.PhaseWaiting
		room.normalizePhaseForNicknameGate()
		if room.state.Phase != domain.PhaseWaiting {
			t.Errorf("Phase = %v, want %v", room.state.Phase, domain.PhaseWaiting)
		}
	})

	t.Run("playing resets to waiting when not all ready", func(t *testing.T) {
		room := &Room{
			state:       NewGameState("TEST"),
			connections: make(map[string]*PlayerConn),
			usedNames:   make(map[string]bool),
			logger:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		room.state.Phase = domain.PhasePlaying
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: false}
		room.connections["p1"] = &PlayerConn{}

		room.normalizePhaseForNicknameGate()
		if room.state.Phase != domain.PhaseWaiting {
			t.Errorf("Phase should reset to waiting, got %v", room.state.Phase)
		}
	})
}

func TestTransitionPhaseIfNeeded(t *testing.T) {
	t.Parallel()

	t.Run("playing without tick starts tick", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
		}
		room.state.Phase = domain.PhasePlaying
		room.tickCancel = nil

		room.transitionPhaseIfNeeded()
	})

	t.Run("playing with active tick does nothing", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
		}
		room.state.Phase = domain.PhasePlaying
		room.tickCancel = func() {}

		room.transitionPhaseIfNeeded()
		// tickCancel should not be replaced
		if room.tickCancel == nil {
			t.Error("tickCancel should not be nil after transitionPhaseIfNeeded")
		}
	})

	t.Run("non-playing phase does nothing", func(t *testing.T) {
		room := &Room{state: NewGameState("TEST")}
		room.state.Phase = domain.PhaseWaiting
		room.transitionPhaseIfNeeded()
	})
}


