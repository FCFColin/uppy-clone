package game

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestDeterministicTick_SameSeedSameHash(t *testing.T) {
	t.Parallel()

	tickCount := 200
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	hash1 := deterministicTickHash(t, tickCount, fixedTime)
	hash2 := deterministicTickHash(t, tickCount, fixedTime)

	if hash1 != hash2 {
		t.Fatalf("deterministic tick mismatch (seed=42, ticks=%d):\n  run1: %s\n  run2: %s", tickCount, hash1, hash2)
	}
}

func deterministicTickHash(t *testing.T, tickCount int, fixedTime time.Time) string {
	t.Helper()

	r := NewRoom("DET", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	t.Cleanup(r.Close)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rng = newSeededRNG(42)
	r.state = NewGameState("DET", 42, r.rng)
	r.state.Phase = domain.PhasePlaying
	r.state.Players["p1"] = &domain.PlayerState{
		ID:          "p1",
		PlayerIndex: 0,
		Nickname:    "Player1",
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 256)}
	r.usedNames["Player1"] = true
	r.state.SessionID = "det-test"

	for i := 0; i < tickCount; i++ {
		r.tickOnce(fixedTime)
		if r.state.Phase != domain.PhasePlaying {
			break
		}
	}

	data, err := json.Marshal(r.state)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

func TestDeterministicTick_DifferentSeedDifferentHash(t *testing.T) {
	t.Parallel()

	tickCount := 200
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	hash1 := deterministicTickHashWithSeed(t, tickCount, fixedTime, 42)
	hash2 := deterministicTickHashWithSeed(t, tickCount, fixedTime, 99)

	if hash1 == hash2 {
		t.Fatalf("different seeds produced same hash (seed1=42, seed2=99, ticks=%d)", tickCount)
	}
}

func deterministicTickHashWithSeed(t *testing.T, tickCount int, fixedTime time.Time, seed int64) string {
	t.Helper()

	r := NewRoom("DET", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	t.Cleanup(r.Close)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rng = newSeededRNG(seed)
	r.state = NewGameState("DET", seed, r.rng)
	r.state.Phase = domain.PhasePlaying
	r.state.Players["p1"] = &domain.PlayerState{
		ID:          "p1",
		PlayerIndex: 0,
		Nickname:    "Player1",
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 256)}
	r.usedNames["Player1"] = true
	r.state.SessionID = "det-test"

	for i := 0; i < tickCount; i++ {
		r.tickOnce(fixedTime)
		if r.state.Phase != domain.PhasePlaying {
			break
		}
	}

	data, err := json.Marshal(r.state)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
