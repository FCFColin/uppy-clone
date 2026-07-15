package game

import (
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"pgregory.net/rapid"
)

func TestState_NewGameStateValidInit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", 0, rng)
		if state == nil ||
			state.Phase != domain.PhaseWaiting ||
			state.Balloon.X != 0.5 ||
			state.Balloon.Y != 0.95 ||
			len(state.Players) != 0 ||
			len(state.RestartVotes) != 0 ||
			state.Wind == 0 {
			t.Fatal("invalid initial game state")
		}
	})
}

func TestState_SerializeStateRoundtrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", 0, rng)
		state.Phase = domain.PhasePlaying
		state.Balloon.Score = int(rng.IntN(100))

		data, err := SerializeState(state)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := DeserializeState(data)
		if err != nil {
			t.Fatal(err)
		}
		if restored.Phase != state.Phase ||
			restored.Balloon.X != state.Balloon.X ||
			restored.Balloon.Y != state.Balloon.Y ||
			restored.TickCount != state.TickCount ||
			restored.LobbyCode != state.LobbyCode ||
			restored.Balloon.Score != state.Balloon.Score {
			t.Fatal("roundtrip mismatch")
		}
	})
}

func TestState_ResetGameEntitiesKeepsPlayerCount(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", 0, rng)
		state.Players["p1"] = &domain.PlayerState{ID: "p1"}
		state.Players["p2"] = &domain.PlayerState{ID: "p2"}
		countBefore := len(state.Players)
		ResetGameEntities(state, RandomSpawnTimer(rng), rng)
		if len(state.Players) != countBefore {
			t.Fatal("player count changed after reset")
		}
	})
}

func TestState_InitWindNoPanic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", 0, rng)
		initWind(state, rng)
		if state.Wind < -protocol.WindClamp || state.Wind > protocol.WindClamp {
			t.Fatal("wind out of bounds")
		}
	})
}

func TestState_DomainAddRemovePlayer(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", 0, rng)
		initialCount := len(state.Players)

		if err := state.AddPlayer(&domain.PlayerState{ID: "p1"}); err != nil {
			t.Fatal(err)
		}
		if len(state.Players) != initialCount+1 {
			t.Fatal("expected 1 added player")
		}
		if err := state.AddPlayer(&domain.PlayerState{ID: "p2"}); err != nil {
			t.Fatal(err)
		}
		if len(state.Players) != initialCount+2 {
			t.Fatal("expected 2 added players")
		}
		state.RemovePlayer("p1")
		if len(state.Players) != initialCount+1 {
			t.Fatal("expected 1 remaining player after removal")
		}
	})
}

func TestState_DomainGameStateIsGameOver(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", 0, rng)
		if state.IsGameOver() {
			t.Fatal("new game should not be game over")
		}
		state.Phase = domain.PhaseEnded
		if !state.IsGameOver() {
			t.Fatal("ended game should be game over")
		}
	})
}

func TestState_DeserializeStateNilMaps(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rapid.Int64().Draw(t, "seed")
		data := []byte(`{"lobbyCode":"PROP","phase":"playing"}`)
		state, err := DeserializeState(data)
		if err != nil {
			t.Fatal(err)
		}
		if state.Players == nil || state.RestartVotes == nil {
			t.Fatal("deserialized maps should not be nil")
		}
	})
}
