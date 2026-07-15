package protocol

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// shortNickGenProp generates nicknames with 0–20 arbitrary runes (≤80 bytes,
// well under the uint8 byte-length limit enforced by the encoder).
var shortNickGenProp = rapid.Custom(func(t *rapid.T) string {
	n := rapid.IntRange(0, 20).Draw(t, "nickLen")
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = rapid.Rune().Draw(t, "rune")
	}
	return string(runes)
})

var playerStateGenProp = rapid.Custom(func(t *rapid.T) PlayerState {
	return PlayerState{
		PlayerIndex:       rapid.Uint16().Draw(t, "playerIndex"),
		CooldownMs:        rapid.Uint32().Draw(t, "cooldownMs"),
		Palette:           rapid.Uint32().Draw(t, "palette"),
		ScoreContribution: rapid.Uint32().Draw(t, "scoreContribution"),
		Nickname:          shortNickGenProp.Draw(t, "nickname"),
	}
})

var rippleGenProp = rapid.Custom(func(t *rapid.T) Ripple {
	return Ripple{
		PlayerIndex: rapid.Uint16().Draw(t, "playerIndex"),
		X:           rapid.Float32().Draw(t, "x"),
		Y:           rapid.Float32().Draw(t, "y"),
	}
})

// TestProtocol_SnapshotRoundTripPreservesAllFields: Encoding a snapshot and then
// decoding it preserves all fields (phase, tickCount, score, balloon, bird, ghost,
// players, ripples, wind). Inactive bird/ghost have their X/Y/RepelTimer zeroed
// because those fields are not transmitted when the active flag is false.
func TestProtocol_SnapshotRoundTripPreservesAllFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		phase := rapid.SampledFrom([]GamePhase{PhaseWaiting, PhaseCountdown, PhasePlaying, PhaseEnded}).Draw(t, "phase")
		tickCount := rapid.Uint32().Draw(t, "tickCount")
		score := rapid.Uint32().Draw(t, "score")
		balloon := BalloonState{
			X:  rapid.Float32().Draw(t, "balloonX"),
			Y:  rapid.Float32().Draw(t, "balloonY"),
			Vx: rapid.Float32().Draw(t, "balloonVx"),
			Vy: rapid.Float32().Draw(t, "balloonVy"),
		}
		birdActive := rapid.Bool().Draw(t, "birdActive")
		bird := BirdState{Active: birdActive}
		if birdActive {
			bird.X = rapid.Float32().Draw(t, "birdX")
			bird.Y = rapid.Float32().Draw(t, "birdY")
		}
		ghostActive := rapid.Bool().Draw(t, "ghostActive")
		ghost := GhostState{Active: ghostActive}
		if ghostActive {
			ghost.X = rapid.Float32().Draw(t, "ghostX")
			ghost.Y = rapid.Float32().Draw(t, "ghostY")
			ghost.RepelTimer = rapid.Uint16().Draw(t, "repelTimer")
		}
		players := rapid.SliceOf(playerStateGenProp).Draw(t, "players")
		if len(players) > 200 {
			return
		}
		ripples := rapid.SliceOf(rippleGenProp).Draw(t, "ripples")
		if len(ripples) > 200 {
			return
		}
		wind := rapid.Float64().Draw(t, "wind")

		data := EncodeSnapshot(phase, tickCount, score, balloon, bird, ghost, players, ripples, wind)
		ds, ok := decodeSnapshot(data)
		if !ok {
			t.Fatalf("decodeSnapshot failed for data len=%d", len(data))
		}
		if ds.tickCount != tickCount {
			t.Errorf("tickCount = %d, want %d", ds.tickCount, tickCount)
		}
		if ds.score != score {
			t.Errorf("score = %d, want %d", ds.score, score)
		}
		if ds.phase != phase {
			t.Errorf("phase = %q, want %q", ds.phase, phase)
		}
		if ds.balloon != balloon {
			t.Errorf("balloon = %+v, want %+v", ds.balloon, balloon)
		}
		if ds.bird != bird {
			t.Errorf("bird = %+v, want %+v", ds.bird, bird)
		}
		if ds.ghost != ghost {
			t.Errorf("ghost = %+v, want %+v", ds.ghost, ghost)
		}
		if len(ds.players) != len(players) {
			t.Fatalf("players len = %d, want %d", len(ds.players), len(players))
		}
		for i, p := range players {
			if ds.players[i] != p {
				t.Errorf("players[%d] = %+v, want %+v", i, ds.players[i], p)
			}
		}
		if len(ds.ripples) != len(ripples) {
			t.Fatalf("ripples len = %d, want %d", len(ds.ripples), len(ripples))
		}
		for i, r := range ripples {
			if ds.ripples[i] != r {
				t.Errorf("ripples[%d] = %+v, want %+v", i, ds.ripples[i], r)
			}
		}
		if ds.wind != float32(wind) {
			t.Errorf("wind = %v, want %v", ds.wind, float32(wind))
		}
	})
}

// TestProtocol_TapAcceptedRoundTrip: EncodeTapAccepted then DecodeMessage preserves
// all fields (playerIndex, cooldownMs, balloonX, balloonY).
func TestProtocol_TapAcceptedRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		playerIndex := rapid.Uint16().Draw(t, "playerIndex")
		cooldownMs := rapid.Uint32().Draw(t, "cooldownMs")
		balloonX := rapid.Float32().Draw(t, "balloonX")
		balloonY := rapid.Float32().Draw(t, "balloonY")

		data := EncodeTapAccepted(playerIndex, cooldownMs, balloonX, balloonY)
		msgType, payload := DecodeMessage(data)
		if msgType != MsgTapAccepted {
			t.Fatalf("msgType = 0x%02x, want 0x%02x", msgType, MsgTapAccepted)
		}
		if len(payload) != 14 {
			t.Fatalf("payload len = %d, want 14", len(payload))
		}
		gotPlayerIndex := le.Uint16(payload[0:2])
		gotCooldownMs := le.Uint32(payload[2:6])
		gotBalloonX := math.Float32frombits(le.Uint32(payload[6:10]))
		gotBalloonY := math.Float32frombits(le.Uint32(payload[10:14]))
		if gotPlayerIndex != playerIndex {
			t.Fatalf("playerIndex = %d, want %d", gotPlayerIndex, playerIndex)
		}
		if gotCooldownMs != cooldownMs {
			t.Fatalf("cooldownMs = %d, want %d", gotCooldownMs, cooldownMs)
		}
		if gotBalloonX != balloonX {
			t.Fatalf("balloonX = %v, want %v", gotBalloonX, balloonX)
		}
		if gotBalloonY != balloonY {
			t.Fatalf("balloonY = %v, want %v", gotBalloonY, balloonY)
		}
	})
}

// TestProtocol_PlayerJoinRoundTrip: EncodePlayerJoin then DecodeMessage preserves
// playerIndex, nickname, and palette.
func TestProtocol_PlayerJoinRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		playerIndex := rapid.Uint16().Draw(t, "playerIndex")
		nickname := shortNickGenProp.Draw(t, "nickname")
		palette := rapid.Uint32().Draw(t, "palette")

		data := EncodePlayerJoin(playerIndex, nickname, palette)
		msgType, payload := DecodeMessage(data)
		if msgType != MsgPlayerJoin {
			t.Fatalf("msgType = 0x%02x, want 0x%02x", msgType, MsgPlayerJoin)
		}
		// payload layout: playerIndex(2) + nickLen(1) + nickname + palette(4)
		if len(payload) < 7 {
			t.Fatalf("payload too short: %d", len(payload))
		}
		gotPlayerIndex := le.Uint16(payload[0:2])
		nickLen := int(payload[2])
		endIdx := 3 + nickLen
		if len(payload) < endIdx+4 {
			t.Fatalf("payload truncated: len=%d, need %d", len(payload), endIdx+4)
		}
		gotNickname := string(payload[3:endIdx])
		gotPalette := le.Uint32(payload[endIdx : endIdx+4])
		if gotPlayerIndex != playerIndex {
			t.Fatalf("playerIndex = %d, want %d", gotPlayerIndex, playerIndex)
		}
		if gotNickname != nickname {
			t.Fatalf("nickname = %q, want %q", gotNickname, nickname)
		}
		if gotPalette != palette {
			t.Fatalf("palette = %d, want %d", gotPalette, palette)
		}
	})
}
