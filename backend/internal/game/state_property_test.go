package game

import (
	"testing"
	"testing/quick"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestNewGameState_ValidInit(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		return state != nil &&
			state.Phase == domain.PhaseWaiting &&
			state.Balloon.X == 0.5 &&
			state.Balloon.Y == 0.95 &&
			len(state.Players) == 0 &&
			len(state.RestartVotes) == 0 &&
			state.Wind != 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestSerializeStateRoundtrip(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		state.Phase = domain.PhasePlaying
		state.Balloon.Score = int(rng.IntN(100))

		data, err := SerializeState(state)
		if err != nil {
			return false
		}
		restored, err := DeserializeState(data)
		if err != nil {
			return false
		}
		return restored.Phase == state.Phase &&
			restored.Balloon.X == state.Balloon.X &&
			restored.Balloon.Y == state.Balloon.Y &&
			restored.TickCount == state.TickCount &&
			restored.LobbyCode == state.LobbyCode &&
			restored.Balloon.Score == state.Balloon.Score
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestResetGameEntities_KeepsPlayerCount(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		state.Players["p1"] = &domain.PlayerState{ID: "p1"}
		state.Players["p2"] = &domain.PlayerState{ID: "p2"}
		countBefore := len(state.Players)
		ResetGameEntities(state, RandomSpawnTimer(rng), rng)
		return len(state.Players) == countBefore
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestInitWind_NoPanic(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		initWind(state, rng)
		return state.Wind >= -protocol.WindClamp && state.Wind <= protocol.WindClamp
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestDomainAddRemovePlayer(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		initialCount := len(state.Players)

		state.AddPlayer(&domain.PlayerState{ID: "p1"})
		if len(state.Players) != initialCount+1 {
			return false
		}
		state.AddPlayer(&domain.PlayerState{ID: "p2"})
		if len(state.Players) != initialCount+2 {
			return false
		}
		state.RemovePlayer("p1")
		return len(state.Players) == initialCount+1
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestDomainGameState_IsGameOver(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		if state.IsGameOver() {
			return false
		}
		state.Phase = domain.PhaseEnded
		return state.IsGameOver()
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestDeserializeState_NilMaps(t *testing.T) {
	f := func(seed int64) bool {
		data := []byte(`{"lobbyCode":"PROP","phase":"playing"}`)
		state, err := DeserializeState(data)
		if err != nil {
			return false
		}
		return state.Players != nil && state.RestartVotes != nil
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
