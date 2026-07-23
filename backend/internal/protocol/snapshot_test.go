package protocol

import (
	"math"
	"testing"
)

type decodedSnapshot struct {
	msgType   byte
	tickCount uint32
	score     uint32
	phase     GamePhase
	balloon   BalloonState
	bird      BirdState
	ghost     GhostState
	players   []PlayerState
	ripples   []Ripple
	wind      float32
}

func decodeSnapshot(data []byte) (decodedSnapshot, bool) {
	if len(data) < 1 || data[0] != MsgSnapshot {
		return decodedSnapshot{}, false
	}
	o := 1
	if len(data) < o+9 {
		return decodedSnapshot{}, false
	}
	ds := decodedSnapshot{msgType: data[0]}
	ds.tickCount = le.Uint32(data[o:])
	o += 4
	ds.score = le.Uint32(data[o:])
	o += 4
	switch data[o] {
	case PhaseCodePlaying:
		ds.phase = PhasePlaying
	case PhaseCodeEnded:
		ds.phase = PhaseEnded
	case PhaseCodeCountdown:
		ds.phase = PhaseCountdown
	default:
		ds.phase = PhaseWaiting
	}
	o++

	if len(data) < o+16 {
		return decodedSnapshot{}, false
	}
	ds.balloon.X = math.Float32frombits(le.Uint32(data[o:]))
	o += 4
	ds.balloon.Y = math.Float32frombits(le.Uint32(data[o:]))
	o += 4
	ds.balloon.Vx = math.Float32frombits(le.Uint32(data[o:]))
	o += 4
	ds.balloon.Vy = math.Float32frombits(le.Uint32(data[o:]))
	o += 4

	if len(data) < o+1 {
		return decodedSnapshot{}, false
	}
	ds.bird.Active = data[o] == 1
	o++
	if ds.bird.Active {
		if len(data) < o+8 {
			return decodedSnapshot{}, false
		}
		ds.bird.X = math.Float32frombits(le.Uint32(data[o:]))
		o += 4
		ds.bird.Y = math.Float32frombits(le.Uint32(data[o:]))
		o += 4
	}

	if len(data) < o+1 {
		return decodedSnapshot{}, false
	}
	ds.ghost.Active = data[o] == 1
	o++
	if ds.ghost.Active {
		if len(data) < o+10 {
			return decodedSnapshot{}, false
		}
		ds.ghost.X = math.Float32frombits(le.Uint32(data[o:]))
		o += 4
		ds.ghost.Y = math.Float32frombits(le.Uint32(data[o:]))
		o += 4
		ds.ghost.RepelTimer = le.Uint16(data[o:])
		o += 2
	}

	if len(data) < o+1 {
		return decodedSnapshot{}, false
	}
	playerCount := int(data[o])
	o++
	ds.players = make([]PlayerState, 0, playerCount)
	for i := 0; i < playerCount; i++ {
		if len(data) < o+15 {
			return decodedSnapshot{}, false
		}
		p := PlayerState{}
		p.PlayerIndex = le.Uint16(data[o:])
		o += 2
		p.CooldownMs = le.Uint32(data[o:])
		o += 4
		p.Palette = le.Uint32(data[o:])
		o += 4
		p.ScoreContribution = le.Uint32(data[o:])
		o += 4
		nickLen := int(data[o])
		o++
		if len(data) < o+nickLen {
			return decodedSnapshot{}, false
		}
		p.Nickname = string(data[o : o+nickLen])
		o += nickLen
		ds.players = append(ds.players, p)
	}

	if len(data) < o+1 {
		return decodedSnapshot{}, false
	}
	rippleCount := int(data[o])
	o++
	ds.ripples = make([]Ripple, 0, rippleCount)
	for i := 0; i < rippleCount; i++ {
		if len(data) < o+10 {
			return decodedSnapshot{}, false
		}
		r := Ripple{}
		r.PlayerIndex = le.Uint16(data[o:])
		o += 2
		r.X = math.Float32frombits(le.Uint32(data[o:]))
		o += 4
		r.Y = math.Float32frombits(le.Uint32(data[o:]))
		o += 4
		ds.ripples = append(ds.ripples, r)
	}

	if len(data) < o+4 {
		return decodedSnapshot{}, false
	}
	ds.wind = math.Float32frombits(le.Uint32(data[o:]))

	return ds, true
}

func TestEncodeSnapshot_RoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		phase     GamePhase
		tickCount uint32
		score     uint32
		balloon   BalloonState
		bird      BirdState
		ghost     GhostState
		players   []PlayerState
		ripples   []Ripple
		wind      float64
	}{
		{
			name:      "Full",
			phase:     PhasePlaying,
			tickCount: 12345,
			score:     9999,
			balloon:   BalloonState{X: 0.5, Y: 0.95, Vx: -0.02, Vy: 0.01},
			bird:      BirdState{X: 0.3, Y: 0.4, Active: true},
			ghost:     GhostState{X: 0.6, Y: 0.5, Active: true, RepelTimer: 42},
			players: []PlayerState{
				{PlayerIndex: 0, CooldownMs: 1000, Palette: 1, ScoreContribution: 50, Nickname: "Alice"},
				{PlayerIndex: 1, CooldownMs: 500, Palette: 2, ScoreContribution: 30, Nickname: "Bob"},
			},
			ripples: []Ripple{
				{PlayerIndex: 0, X: 0.5, Y: 0.5},
				{PlayerIndex: 1, X: 0.3, Y: 0.7},
			},
			wind: 0.15,
		},
		{
			name:    "InactiveBirdGhost",
			phase:   PhaseWaiting,
			balloon: BalloonState{X: 0.5, Y: 0.5},
		},
		{
			name:      "UnicodeNickname",
			phase:     PhasePlaying,
			tickCount: 99,
			score:     500,
			balloon:   BalloonState{X: 0.1, Y: 0.2, Vx: 0.3, Vy: 0.4},
			players: []PlayerState{
				{PlayerIndex: 5, CooldownMs: 2000, Palette: 7, ScoreContribution: 100, Nickname: "快乐气球🎮"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := EncodeSnapshot(c.phase, c.tickCount, c.score, c.balloon, c.bird, c.ghost, c.players, c.ripples, c.wind)
			ds, ok := decodeSnapshot(data)
			if !ok {
				t.Fatalf("decodeSnapshot failed for data len=%d", len(data))
			}
			if ds.msgType != MsgSnapshot {
				t.Errorf("msgType = 0x%02x, want 0x%02x", ds.msgType, MsgSnapshot)
			}
			if ds.tickCount != c.tickCount {
				t.Errorf("tickCount = %d, want %d", ds.tickCount, c.tickCount)
			}
			if ds.score != c.score {
				t.Errorf("score = %d, want %d", ds.score, c.score)
			}
			if ds.phase != c.phase {
				t.Errorf("phase = %q, want %q", ds.phase, c.phase)
			}
			if ds.balloon != c.balloon {
				t.Errorf("balloon = %+v, want %+v", ds.balloon, c.balloon)
			}
			if ds.bird != c.bird {
				t.Errorf("bird = %+v, want %+v", ds.bird, c.bird)
			}
			if ds.ghost != c.ghost {
				t.Errorf("ghost = %+v, want %+v", ds.ghost, c.ghost)
			}
			if len(ds.players) != len(c.players) {
				t.Fatalf("players len = %d, want %d", len(ds.players), len(c.players))
			}
			for i, p := range c.players {
				if ds.players[i] != p {
					t.Errorf("players[%d] = %+v, want %+v", i, ds.players[i], p)
				}
			}
			if len(ds.ripples) != len(c.ripples) {
				t.Fatalf("ripples len = %d, want %d", len(ds.ripples), len(c.ripples))
			}
			for i, r := range c.ripples {
				if ds.ripples[i] != r {
					t.Errorf("ripples[%d] = %+v, want %+v", i, ds.ripples[i], r)
				}
			}
			if ds.wind != float32(c.wind) {
				t.Errorf("wind = %v, want %v", ds.wind, float32(c.wind))
			}
		})
	}
}

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


