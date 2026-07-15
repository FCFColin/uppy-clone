package protocol

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"
)

func TestProtocol_EncodeSnapshotSizeMatchesCalc(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x := rapid.Float32().Draw(t, "x")
		y := rapid.Float32().Draw(t, "y")
		vy := rapid.Float32().Draw(t, "vy")
		vx := rapid.Float32().Draw(t, "vx")
		tickCount := rapid.Uint32().Draw(t, "tickCount")
		score := rapid.Uint32().Draw(t, "score")
		balloon := BalloonState{X: x, Y: y, Vy: vy, Vx: vx}
		bird := BirdState{X: 0.3, Y: 0.4, Active: true}
		ghost := GhostState{X: 0.6, Y: 0.5, Active: true, RepelTimer: 10}
		players := []PlayerState{
			{PlayerIndex: 0, CooldownMs: 1000, Palette: 1, ScoreContribution: 50, Nickname: "test"},
		}
		ripples := []Ripple{
			{PlayerIndex: 0, X: 0.5, Y: 0.5},
		}
		data := EncodeSnapshot(PhasePlaying, tickCount, score, balloon, bird, ghost, players, ripples, 0.3)
		expected := calcSnapshotSize(bird, ghost, players, ripples)
		if len(data) != expected {
			t.Fatalf("expected size %d, got %d", expected, len(data))
		}
	})
}

func TestProtocol_EncodeSnapshotEmptySlices(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x := rapid.Float32().Draw(t, "x")
		y := rapid.Float32().Draw(t, "y")
		vy := rapid.Float32().Draw(t, "vy")
		vx := rapid.Float32().Draw(t, "vx")
		balloon := BalloonState{X: x, Y: y, Vy: vy, Vx: vx}
		bird := BirdState{}
		ghost := GhostState{}
		data := EncodeSnapshot(PhaseWaiting, 0, 0, balloon, bird, ghost, nil, nil, 0)
		if len(data) <= 0 || data[0] != MsgSnapshot {
			t.Fatal("unexpected snapshot data")
		}
	})
}

func TestProtocol_EncodeSnapshotDifferentInputsDiffer(t *testing.T) {
	balloon1 := BalloonState{X: 0.5, Y: 0.95, Vy: 0.01, Vx: -0.02}
	balloon2 := BalloonState{X: 0.6, Y: 0.85, Vy: 0.02, Vx: -0.03}
	bird := BirdState{X: 0.3, Y: 0.4, Active: true}
	ghost := GhostState{X: 0.6, Y: 0.5, Active: true, RepelTimer: 10}
	players := []PlayerState{
		{PlayerIndex: 0, CooldownMs: 1000, Palette: 1, ScoreContribution: 50, Nickname: "test"},
	}
	ripples := []Ripple{
		{PlayerIndex: 0, X: 0.5, Y: 0.5},
	}

	data1 := EncodeSnapshot(PhasePlaying, 42, 100, balloon1, bird, ghost, players, ripples, 0.3)
	data2 := EncodeSnapshot(PhasePlaying, 42, 100, balloon2, bird, ghost, players, ripples, 0.3)
	if bytes.Equal(data1, data2) {
		t.Error("different balloon states should produce different snapshots")
	}

	data3 := EncodeSnapshot(PhasePlaying, 42, 100, balloon1, bird, ghost, players, ripples, 0.3)
	data4 := EncodeSnapshot(PhasePlaying, 43, 100, balloon1, bird, ghost, players, ripples, 0.3)
	if bytes.Equal(data3, data4) {
		t.Error("different tick counts should produce different snapshots")
	}
}

func TestProtocol_CalcSnapshotSizeNonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		active := rapid.Bool().Draw(t, "active")
		// misc-008: Use wider range including edge cases (0, 1, large values)
		// instead of playerCount%10 which only tested 0-9.
		playerCount := rapid.IntRange(0, 255).Draw(t, "playerCount")
		rippleCount := rapid.IntRange(0, 255).Draw(t, "rippleCount")
		bird := BirdState{Active: active}
		players := make([]PlayerState, playerCount)
		for i := range players {
			players[i] = PlayerState{PlayerIndex: uint16(i), Nickname: "p"}
		}
		ripples := make([]Ripple, rippleCount)
		size := calcSnapshotSize(bird, GhostState{}, players, ripples)
		if size <= 0 {
			t.Fatal("expected size > 0")
		}
		// Verify EncodeSnapshot produces exactly calcSnapshotSize bytes.
		data := EncodeSnapshot(PhaseWaiting, 0, 0, BalloonState{}, bird, GhostState{}, players, ripples, 0)
		if len(data) != size {
			t.Fatalf("encoded size %d != calculated size %d", len(data), size)
		}
	})
}
