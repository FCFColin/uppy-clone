package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// ─── EncodeSnapshot ──────────────────────────────────────────────────

func TestEncodeSnapshot_BasicFormat(t *testing.T) {
	balloon := BalloonState{X: 0.5, Y: 0.95, Vy: 0.01, Vx: -0.02}
	bird := BirdState{X: 0.3, Y: 0.4, Active: true}
	ghost := GhostState{X: 0.6, Y: 0.5, Active: true, RepelTimer: 10}
	players := []PlayerState{
		{PlayerIndex: 0, CooldownMs: 1000, Palette: 1, ScoreContribution: 50, Nickname: "test"},
	}
	ripples := []Ripple{
		{PlayerIndex: 0, X: 0.5, Y: 0.5},
	}

	data := EncodeSnapshot(PhasePlaying, 42, 100, balloon, bird, ghost, players, ripples, 0.3)

	if len(data) == 0 {
		t.Fatal("EncodeSnapshot 应返回非空数据")
	}
	if data[0] != MsgSnapshot {
		t.Fatalf("首字节应为 MsgSnapshot=0x01，got=0x%02x", data[0])
	}

	// 验证 tickCount（偏移 1，uint32 LE）
	tickCount := binary.LittleEndian.Uint32(data[1:5])
	if tickCount != 42 {
		t.Fatalf("tickCount 不匹配: got=%d, want=42", tickCount)
	}

	// 验证 score（偏移 5，uint32 LE）
	score := binary.LittleEndian.Uint32(data[5:9])
	if score != 100 {
		t.Fatalf("score 不匹配: got=%d, want=100", score)
	}

	// 验证 phaseCode（偏移 9）
	if data[9] != PhaseCodePlaying {
		t.Fatalf("phaseCode 不匹配: got=%d, want=%d", data[9], PhaseCodePlaying)
	}
}

// ─── EncodeTapAccepted ───────────────────────────────────────────────

func TestEncodeTapAccepted(t *testing.T) {
	data := EncodeTapAccepted(3, 2000, 0.5, 0.3)

	if data[0] != MsgTapAccepted {
		t.Fatalf("首字节应为 MsgTapAccepted=0x04，got=0x%02x", data[0])
	}
	playerIndex := binary.LittleEndian.Uint16(data[1:3])
	if playerIndex != 3 {
		t.Fatalf("playerIndex 不匹配: got=%d, want=3", playerIndex)
	}
	cooldownMs := binary.LittleEndian.Uint32(data[3:7])
	if cooldownMs != 2000 {
		t.Fatalf("cooldownMs 不匹配: got=%d, want=2000", cooldownMs)
	}
}

// ─── EncodeTapRejected ───────────────────────────────────────────────

func TestEncodeSingleByteMessages(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		wantMsg  byte
	}{
		{"Pong", EncodePong(), MsgPong},
		{"TapRejected", EncodeTapRejected(), MsgTapRejected},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if len(c.data) != 1 {
				t.Fatalf("expected 1 byte, got %d", len(c.data))
			}
			if c.data[0] != c.wantMsg {
				t.Fatalf("first byte = 0x%02x, want 0x%02x", c.data[0], c.wantMsg)
			}
		})
	}
}

// ─── EncodeGameStateChange ───────────────────────────────────────────

func TestEncodeGameStateChange_AllPhases(t *testing.T) {
	phases := []GamePhase{PhaseWaiting, PhasePlaying, PhaseEnded}
	for _, phase := range phases {
		data := EncodeGameStateChange(phase)
		if len(data) != 2 {
			t.Fatalf("EncodeGameStateChange 应为 2 字节，got=%d", len(data))
		}
		if data[0] != MsgGameStateChange {
			t.Fatalf("首字节应为 MsgGameStateChange=0x06，got=0x%02x", data[0])
		}
		if data[1] != PhaseToCode(phase) {
			t.Fatalf("phaseCode 不匹配: got=%d, want=%d", data[1], PhaseToCode(phase))
		}
	}
}

func TestEncodeGameStateChange_CountdownWithRemaining(t *testing.T) {
	data := EncodeGameStateChange(PhaseCountdown, 3000)
	if len(data) != 6 {
		t.Fatalf("countdown EncodeGameStateChange 应为 6 字节，got=%d", len(data))
	}
	if data[0] != MsgGameStateChange || data[1] != PhaseCodeCountdown {
		t.Fatalf("unexpected header: %v", data[:2])
	}
	remaining := binary.LittleEndian.Uint32(data[2:6])
	if remaining != 3000 {
		t.Fatalf("countdownRemainingMs 不匹配: got=%d, want=3000", remaining)
	}
}

func TestEncodeGameStateChangeEnded_WithReason(t *testing.T) {
	data := EncodeGameStateChangeEnded(EndReasonBird)
	if len(data) != 3 {
		t.Fatalf("ended EncodeGameStateChangeEnded 应为 3 字节，got=%d", len(data))
	}
	if data[0] != MsgGameStateChange || data[1] != PhaseCodeEnded || data[2] != EndReasonBird {
		t.Fatalf("unexpected payload: %v", data)
	}
}

// ─── DecodeTap ───────────────────────────────────────────────────────

func TestDecodeTap(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		wantX   float32
		wantY   float32
		wantOK  bool
	}{
		{"valid_0.5_0.3", encodeFloat32Pair(0.5, 0.3), 0.5, 0.3, true},
		{"valid_0.75_0.25", encodeFloat32Pair(0.75, 0.25), 0.75, 0.25, true},
		{"too_short", []byte{0x01, 0x02}, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tapX, tapY, ok := DecodeTap(c.payload)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && (tapX != c.wantX || tapY != c.wantY) {
				t.Fatalf("got (%v, %v), want (%v, %v)", tapX, tapY, c.wantX, c.wantY)
			}
		})
	}
}

func encodeFloat32Pair(x, y float32) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, le, x)
	_ = binary.Write(&buf, le, y)
	return buf.Bytes()
}

func TestDecodeNicknamePayload(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		wantNick string
		wantOK  bool
	}{
		{"valid", append([]byte{5}, []byte("hello")...), "hello", true},
		{"empty", nil, "", false},
		{"zero_length", []byte{0}, "", false},
		{"truncated", []byte{5, 'a', 'b'}, "", false},
		{"negative_length", []byte{255, 'x'}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nick, ok := DecodeNicknamePayload(c.payload)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if nick != c.wantNick {
				t.Fatalf("nick = %q, want %q", nick, c.wantNick)
			}
		})
	}
}

// ─── DecodeMessage ───────────────────────────────────────────────────

func TestDecodeMessage(t *testing.T) {
	msgType, payload := DecodeMessage([]byte{MsgTap, 0x01, 0x02})
	if msgType != MsgTap {
		t.Fatalf("消息类型不匹配: got=0x%02x, want=0x%02x", msgType, MsgTap)
	}
	if len(payload) != 2 {
		t.Fatalf("payload 长度不匹配: got=%d, want=2", len(payload))
	}
}

func TestDecodeMessage_Empty(t *testing.T) {
	msgType, payload := DecodeMessage([]byte{})
	if msgType != 0 {
		t.Fatalf("空消息应返回类型 0，got=0x%02x", msgType)
	}
	if payload != nil {
		t.Fatal("空消息 payload 应为 nil")
	}
}

// ─── WSMessageTypeName ──────────────────────────────────────────────

func TestWSMessageTypeName_AllCases(t *testing.T) {
	cases := map[byte]string{
		MsgTap:         "tap",
		MsgSetNickname: "set_nickname",
		MsgRestartVote: "restart_vote",
		MsgPing:        "ping",
		0xFF:           "unknown",
	}
	for msgType, want := range cases {
		if got := WSMessageTypeName(msgType); got != want {
			t.Fatalf("WSMessageTypeName(0x%02x) = %q, want %q", msgType, got, want)
		}
	}
}

// ─── PhaseToCode ────────────────────────────────────────────────────

func TestPhaseToCode(t *testing.T) {
	cases := []struct {
		name  string
		phase GamePhase
		code  uint8
	}{
		{"waiting", PhaseWaiting, PhaseCodeWaiting},
		{"countdown", PhaseCountdown, PhaseCodeCountdown},
		{"playing", PhasePlaying, PhaseCodePlaying},
		{"ended", PhaseEnded, PhaseCodeEnded},
		{"unknown", GamePhase("unknown"), PhaseCodeWaiting},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PhaseToCode(c.phase); got != c.code {
				t.Fatalf("PhaseToCode(%q) = %d, want %d", c.phase, got, c.code)
			}
		})
	}
}

// ─── EncodePlayerJoin / EncodePlayerLeave ────────────────────────────

func TestEncodePlayerJoin(t *testing.T) {
	data := EncodePlayerJoin(5, "TestPlayer", 3)
	if data[0] != MsgPlayerJoin {
		t.Fatalf("first byte should be MsgPlayerJoin=0x02, got=0x%02x", data[0])
	}
}

func TestEncodePlayerLeave(t *testing.T) {
	data := EncodePlayerLeave(5)
	if data[0] != MsgPlayerLeave {
		t.Fatalf("first byte should be MsgPlayerLeave=0x03, got=0x%02x", data[0])
	}
	playerIndex := binary.LittleEndian.Uint16(data[1:3])
	if playerIndex != 5 {
		t.Fatalf("playerIndex mismatch: got=%d, want=5", playerIndex)
	}
}

func TestEncodeRestartStatus(t *testing.T) {
	data := EncodeRestartStatus(2, 5, 30000)
	if data[0] != MsgRestartStatus {
		t.Fatalf("first byte should be MsgRestartStatus=0x07, got=0x%02x", data[0])
	}
	if data[1] != 2 {
		t.Fatalf("yesVotes mismatch: got=%d, want=2", data[1])
	}
	if data[2] != 5 {
		t.Fatalf("totalPlayers mismatch: got=%d, want=5", data[2])
	}
}

// ─── EncodeSnapshot round-trip tests (misc-007) ──────────────────────

// decodedSnapshot holds fields extracted from a binary snapshot for round-trip
// verification. It mirrors the frontend's DecodedSnapshot interface.
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

// decodeSnapshot manually parses the binary layout produced by EncodeSnapshot.
// This is the server-side mirror of the frontend's decodeSnapshot() function.
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
	// Inline phase decode (CodeToPhase removed as dead production code).
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

	// Balloon: x, y, vx, vy (16 bytes)
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

	// Bird: active flag + optional x, y
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

	// Ghost: active flag + optional x, y, repelTimer
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

	// Players
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

	// Ripples
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

	// Wind
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
		{name: "PhaseWaiting", phase: PhaseWaiting, tickCount: 1, score: 1, balloon: BalloonState{X: 0.5, Y: 0.5}},
		{name: "PhaseCountdown", phase: PhaseCountdown, tickCount: 1, score: 1, balloon: BalloonState{X: 0.5, Y: 0.5}},
		{name: "PhasePlaying", phase: PhasePlaying, tickCount: 1, score: 1, balloon: BalloonState{X: 0.5, Y: 0.5}},
		{name: "PhaseEnded", phase: PhaseEnded, tickCount: 1, score: 1, balloon: BalloonState{X: 0.5, Y: 0.5}},
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

