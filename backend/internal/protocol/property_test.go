package protocol

import (
	"bytes"
	"testing"
	"testing/quick"
)

func TestProtocol_EncodeSnapshotSizeMatchesCalc(t *testing.T) {
	f := func(x, y, vy, vx float32, tickCount uint32, score uint32) bool {
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
		expected := calcSnapshotSize(bird, players, ripples)
		return len(data) == expected
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestProtocol_EncodeSnapshotEmptySlices(t *testing.T) {
	f := func(x, y, vy, vx float32) bool {
		balloon := BalloonState{X: x, Y: y, Vy: vy, Vx: vx}
		bird := BirdState{}
		ghost := GhostState{}
		data := EncodeSnapshot(PhaseWaiting, 0, 0, balloon, bird, ghost, nil, nil, 0)
		return len(data) > 0 && data[0] == MsgSnapshot
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
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
	f := func(active bool, playerCount uint8, rippleCount uint8) bool {
		bird := BirdState{Active: active}
		players := make([]PlayerState, playerCount%10)
		for i := range players {
			players[i] = PlayerState{PlayerIndex: uint16(i), Nickname: "p"}
		}
		ripples := make([]Ripple, rippleCount%10)
		size := calcSnapshotSize(bird, players, ripples)
		return size > 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
