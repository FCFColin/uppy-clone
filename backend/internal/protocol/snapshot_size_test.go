package protocol

import (
	"bytes"
	"testing"
)

// TestProtocol_SnapshotSizeMatchesCalc_TableDriven verifies that EncodeSnapshot
// produces exactly calcSnapshotSize bytes for representative boundary cases.
// Replaces the original rapid property tests (EncodeSnapshotSizeMatchesCalc and
// CalcSnapshotSizeNonNegative) while preserving key boundary coverage.
func TestProtocol_SnapshotSizeMatchesCalc_TableDriven(t *testing.T) {
	balloon := BalloonState{X: 0.5, Y: 0.95, Vx: -0.02, Vy: 0.01}
	tests := []struct {
		name        string
		bird        BirdState
		ghost       GhostState
		playerCount int
		rippleCount int
	}{
		{"empty slices inactive entities", BirdState{}, GhostState{}, 0, 0},
		{"empty slices active bird", BirdState{X: 0.3, Y: 0.4, Active: true}, GhostState{}, 0, 0},
		{"empty slices active ghost", BirdState{}, GhostState{X: 0.6, Y: 0.5, Active: true, RepelTimer: 10}, 0, 0},
		{"one player one ripple", BirdState{Active: true}, GhostState{Active: true, RepelTimer: 5}, 1, 1},
		{"two players", BirdState{Active: true}, GhostState{Active: true}, 2, 0},
		{"many players many ripples", BirdState{Active: true}, GhostState{Active: true}, 10, 5},
		{"max boundary counts", BirdState{Active: true}, GhostState{Active: true}, 255, 255},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			players := make([]PlayerState, tt.playerCount)
			for i := range players {
				players[i] = PlayerState{PlayerIndex: uint16(i), Nickname: "p"}
			}
			ripples := make([]Ripple, tt.rippleCount)
			for i := range ripples {
				ripples[i] = Ripple{PlayerIndex: uint16(i), X: 0.5, Y: 0.5}
			}
			expected := calcSnapshotSize(tt.bird, tt.ghost, players, ripples)
			if expected <= 0 {
				t.Fatalf("calcSnapshotSize returned non-positive: %d", expected)
			}
			data := EncodeSnapshot(PhasePlaying, 42, 100, balloon, tt.bird, tt.ghost, players, ripples, 0.3)
			if len(data) != expected {
				t.Fatalf("encoded size %d != calculated size %d", len(data), expected)
			}
		})
	}
}

// TestProtocol_EncodeSnapshotEmptySlices verifies that encoding with empty/nil
// slices produces a valid snapshot starting with MsgSnapshot.
func TestProtocol_EncodeSnapshotEmptySlices(t *testing.T) {
	balloon := BalloonState{X: 0.5, Y: 0.5, Vx: 0, Vy: 0}
	data := EncodeSnapshot(PhaseWaiting, 0, 0, balloon, BirdState{}, GhostState{}, nil, nil, 0)
	if len(data) <= 0 || data[0] != MsgSnapshot {
		t.Fatal("unexpected snapshot data")
	}
}

// TestProtocol_EncodeSnapshotDifferentInputsDiffer verifies that different
// balloon states and tick counts produce distinct encoded outputs.
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
