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

func TestEncodeSnapshot_BirdInactive(t *testing.T) {
	balloon := BalloonState{X: 0.5, Y: 0.95, Vy: 0, Vx: 0}
	bird := BirdState{Active: false}
	ghost := GhostState{X: 0.6, Y: 0.5, Active: false, RepelTimer: 0}

	data := EncodeSnapshot(PhaseWaiting, 0, 0, balloon, bird, ghost, nil, nil, 0)

	// 鸟 inactive 时只写 1 字节 (0)，不写坐标
	// 偏移: header(10) + balloon(16) = 26 → bird active flag
	if data[26] != 0 {
		t.Fatalf("鸟未激活时 active 标志应为 0，got=%d", data[26])
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

func TestEncodeTapRejected(t *testing.T) {
	data := EncodeTapRejected()

	if len(data) != 1 {
		t.Fatalf("EncodeTapRejected 应为 1 字节，got=%d", len(data))
	}
	if data[0] != MsgTapRejected {
		t.Fatalf("首字节应为 MsgTapRejected=0x05，got=0x%02x", data[0])
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

func TestDecodeTap_Valid(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, le, float32(0.5))
	_ = binary.Write(&buf, le, float32(0.3))

	tapX, tapY, ok := DecodeTap(buf.Bytes())
	if !ok {
		t.Fatal("有效的 tap 消息应解码成功")
	}
	if tapX != 0.5 {
		t.Fatalf("tapX 不匹配: got=%v, want=0.5", tapX)
	}
	if tapY != 0.3 {
		t.Fatalf("tapY 不匹配: got=%v, want=0.3", tapY)
	}
}

func TestDecodeTap_TooShort(t *testing.T) {
	_, _, ok := DecodeTap([]byte{0x01, 0x02})
	if ok {
		t.Fatal("过短的消息应解码失败")
	}
}

func TestDecodeNicknamePayload_Valid(t *testing.T) {
	nick, ok := DecodeNicknamePayload(append([]byte{5}, []byte("hello")...))
	if !ok || nick != "hello" {
		t.Fatalf("DecodeNicknamePayload = (%q, %v), want (hello, true)", nick, ok)
	}
}

func TestDecodeNicknamePayload_Empty(t *testing.T) {
	_, ok := DecodeNicknamePayload(nil)
	if ok {
		t.Fatal("empty payload should fail")
	}
}

func TestDecodeNicknamePayload_ZeroLength(t *testing.T) {
	_, ok := DecodeNicknamePayload([]byte{0})
	if ok {
		t.Fatal("zero nickLen should fail")
	}
}

func TestDecodeNicknamePayload_Truncated(t *testing.T) {
	_, ok := DecodeNicknamePayload([]byte{5, 'a', 'b'})
	if ok {
		t.Fatal("truncated nickname should fail")
	}
}

func TestDecodeNicknamePayload_NegativeLength(t *testing.T) {
	_, ok := DecodeNicknamePayload([]byte{255, 'x'})
	if ok {
		t.Fatal("nickLen > payload should fail")
	}
}

// ─── Round-trip: encode then decode ─────────────────────────────────

func TestRoundTrip_Tap(t *testing.T) {
	// 构造客户端 tap payload（无 msgType 前缀，DecodeMessage 已剥离）
	var buf bytes.Buffer
	_ = binary.Write(&buf, le, float32(0.75))
	_ = binary.Write(&buf, le, float32(0.25))

	tapX, tapY, ok := DecodeTap(buf.Bytes())
	if !ok {
		t.Fatal("round-trip 解码应成功")
	}
	if tapX != 0.75 || tapY != 0.25 {
		t.Fatalf("round-trip 值不匹配: got (%v, %v), want (0.75, 0.25)", tapX, tapY)
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

func TestWSMessageTypeName(t *testing.T) {
	if got := WSMessageTypeName(MsgSetNickname); got != "set_nickname" {
		t.Fatalf("got %q, want set_nickname", got)
	}
}

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

func TestPhaseToCode_RoundTrip(t *testing.T) {
	cases := []struct {
		phase GamePhase
		code  uint8
	}{
		{PhaseWaiting, PhaseCodeWaiting},
		{PhaseCountdown, PhaseCodeCountdown},
		{PhasePlaying, PhaseCodePlaying},
		{PhaseEnded, PhaseCodeEnded},
	}
	for _, c := range cases {
		if got := PhaseToCode(c.phase); got != c.code {
			t.Errorf("PhaseToCode(%q) = %d, want %d", c.phase, got, c.code)
		}
	}
}

func TestPhaseToCode_Unknown(t *testing.T) {
	if got := PhaseToCode(GamePhase("unknown")); got != PhaseCodeWaiting {
		t.Fatalf("unknown phase should map to PhaseCodeWaiting, got %d", got)
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

func TestEncodePong(t *testing.T) {
	data := EncodePong()
	if len(data) != 1 {
		t.Fatalf("EncodePong should be 1 byte, got=%d", len(data))
	}
	if data[0] != MsgPong {
		t.Fatalf("first byte should be MsgPong=0x21, got=0x%02x", data[0])
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

// ─── Benchmarks ──────────────────────────────────────────────────────

func BenchmarkEncodeSnapshot(b *testing.B) {
	balloon := BalloonState{X: 0.5, Y: 0.95, Vy: 0.01, Vx: -0.02}
	bird := BirdState{X: 0.3, Y: 0.4, Active: true}
	ghost := GhostState{X: 0.6, Y: 0.5, Active: true, RepelTimer: 10}
	players := []PlayerState{
		{PlayerIndex: 0, CooldownMs: 1000, Palette: 1, ScoreContribution: 50, Nickname: "test"},
		{PlayerIndex: 1, CooldownMs: 500, Palette: 2, ScoreContribution: 30, Nickname: "player2"},
	}
	ripples := []Ripple{
		{PlayerIndex: 0, X: 0.5, Y: 0.5},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EncodeSnapshot(PhasePlaying, 42, 100, balloon, bird, ghost, players, ripples, 0.3)
	}
}

func BenchmarkEncodeSnapshot_NoPlayers(b *testing.B) {
	balloon := BalloonState{X: 0.5, Y: 0.95, Vy: 0, Vx: 0}
	bird := BirdState{Active: false}
	ghost := GhostState{X: 0.6, Y: 0.5, Active: false, RepelTimer: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EncodeSnapshot(PhaseWaiting, 0, 0, balloon, bird, ghost, nil, nil, 0)
	}
}

func BenchmarkDecodeTap(b *testing.B) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, le, float32(0.5))
	_ = binary.Write(&buf, le, float32(0.3))
	data := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecodeTap(data)
	}
}

func BenchmarkEncodeTapAccepted(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EncodeTapAccepted(3, 2000, 0.5, 0.3)
	}
}

func BenchmarkDecodeMessage(b *testing.B) {
	data := []byte{MsgTap, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecodeMessage(data)
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

func TestEncodeSnapshot_RoundTrip_Full(t *testing.T) {
	phase := PhasePlaying
	tickCount := uint32(12345)
	score := uint32(9999)
	balloon := BalloonState{X: 0.5, Y: 0.95, Vx: -0.02, Vy: 0.01}
	bird := BirdState{X: 0.3, Y: 0.4, Active: true}
	ghost := GhostState{X: 0.6, Y: 0.5, Active: true, RepelTimer: 42}
	players := []PlayerState{
		{PlayerIndex: 0, CooldownMs: 1000, Palette: 1, ScoreContribution: 50, Nickname: "Alice"},
		{PlayerIndex: 1, CooldownMs: 500, Palette: 2, ScoreContribution: 30, Nickname: "Bob"},
	}
	ripples := []Ripple{
		{PlayerIndex: 0, X: 0.5, Y: 0.5},
		{PlayerIndex: 1, X: 0.3, Y: 0.7},
	}
	wind := 0.15

	data := EncodeSnapshot(phase, tickCount, score, balloon, bird, ghost, players, ripples, wind)

	ds, ok := decodeSnapshot(data)
	if !ok {
		t.Fatalf("decodeSnapshot failed for data len=%d", len(data))
	}

	if ds.msgType != MsgSnapshot {
		t.Errorf("msgType = 0x%02x, want 0x%02x", ds.msgType, MsgSnapshot)
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
}

func TestEncodeSnapshot_RoundTrip_InactiveBirdGhost(t *testing.T) {
	phase := PhaseWaiting
	tickCount := uint32(0)
	score := uint32(0)
	balloon := BalloonState{X: 0.5, Y: 0.5, Vx: 0, Vy: 0}
	bird := BirdState{Active: false}
	ghost := GhostState{Active: false}
	wind := 0.0

	data := EncodeSnapshot(phase, tickCount, score, balloon, bird, ghost, nil, nil, wind)

	ds, ok := decodeSnapshot(data)
	if !ok {
		t.Fatalf("decodeSnapshot failed for data len=%d", len(data))
	}

	if ds.bird.Active {
		t.Error("bird should be inactive")
	}
	if ds.ghost.Active {
		t.Error("ghost should be inactive")
	}
	if ds.balloon != balloon {
		t.Errorf("balloon = %+v, want %+v", ds.balloon, balloon)
	}
	if len(ds.players) != 0 {
		t.Errorf("players len = %d, want 0", len(ds.players))
	}
	if len(ds.ripples) != 0 {
		t.Errorf("ripples len = %d, want 0", len(ds.ripples))
	}
}

func TestEncodeSnapshot_RoundTrip_UnicodeNickname(t *testing.T) {
	nick := "快乐气球🎮"
	players := []PlayerState{
		{PlayerIndex: 5, CooldownMs: 2000, Palette: 7, ScoreContribution: 100, Nickname: nick},
	}

	data := EncodeSnapshot(PhasePlaying, 99, 500,
		BalloonState{X: 0.1, Y: 0.2, Vx: 0.3, Vy: 0.4},
		BirdState{Active: false},
		GhostState{Active: false},
		players, nil, 0.0)

	ds, ok := decodeSnapshot(data)
	if !ok {
		t.Fatalf("decodeSnapshot failed")
	}
	if len(ds.players) != 1 {
		t.Fatalf("players len = %d, want 1", len(ds.players))
	}
	if ds.players[0].Nickname != nick {
		t.Errorf("nickname = %q, want %q", ds.players[0].Nickname, nick)
	}
	if ds.players[0].PlayerIndex != 5 {
		t.Errorf("playerIndex = %d, want 5", ds.players[0].PlayerIndex)
	}
}

func TestEncodeSnapshot_RoundTrip_AllPhases(t *testing.T) {
	phases := []GamePhase{PhaseWaiting, PhaseCountdown, PhasePlaying, PhaseEnded}
	balloon := BalloonState{X: 0.5, Y: 0.5, Vx: 0, Vy: 0}

	for _, phase := range phases {
		data := EncodeSnapshot(phase, 1, 1, balloon,
			BirdState{Active: false}, GhostState{Active: false}, nil, nil, 0)

		ds, ok := decodeSnapshot(data)
		if !ok {
			t.Errorf("decodeSnapshot failed for phase %q", phase)
			continue
		}
		if ds.phase != phase {
			t.Errorf("phase = %q, want %q", ds.phase, phase)
		}
	}
}

// ─── Fuzz tests (v2-R-103~106) ───────────────────────────────────────

// FuzzDecodeNicknamePayload ensures DecodeNicknamePayload never panics and
// returns consistent results for arbitrary byte inputs.
// Note: FuzzDecodeTap/FuzzDecodeMessage already exist
// in decode_fuzz_test.go; this adds coverage for the standalone payload decoder.
func FuzzDecodeNicknamePayload(f *testing.F) {
	// Seed corpus: valid, empty, zero-length, truncated, oversized.
	f.Add([]byte{5, 'h', 'e', 'l', 'l', 'o'})
	f.Add([]byte{})
	f.Add([]byte{0})
	f.Add([]byte{3, 'a', 'b'})
	f.Add([]byte{255, 'x'})
	f.Add([]byte{1, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		nickname, ok := DecodeNicknamePayload(data)
		if !ok {
			// On failure, nickname must be empty.
			if nickname != "" {
				t.Fatalf("on failure expected empty nickname, got %q", nickname)
			}
			return
		}
		// On success: data must have at least 1 byte (nickLen) and nickLen > 0.
		if len(data) < 1 {
			t.Fatalf("ok=true but data empty")
		}
		nickLen := int(data[0])
		if nickLen <= 0 {
			t.Fatalf("ok=true but nickLen=%d <= 0", nickLen)
		}
		if len(data) < 1+nickLen {
			t.Fatalf("ok=true but data too short: len=%d, need %d", len(data), 1+nickLen)
		}
		// Nickname bytes must match the slice.
		expected := string(data[1 : 1+nickLen])
		if nickname != expected {
			t.Fatalf("nickname=%q, want %q", nickname, expected)
		}
		if len(nickname) != nickLen {
			t.Fatalf("len(nickname)=%d, want %d", len(nickname), nickLen)
		}
	})
}
