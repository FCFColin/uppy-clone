package protocol

import (
	"bytes"
	"encoding/binary"
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
	phases := []GamePhase{PhaseWaiting, PhaseCountdown, PhasePlaying, PhaseEnded}
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

// ─── DecodeSetNickname ───────────────────────────────────────────────

func TestDecodeSetNickname_Valid(t *testing.T) {
	nick := "快乐的气球"
	nickBytes := []byte(nick)

	var buf bytes.Buffer
	buf.WriteByte(MsgSetNickname)
	buf.WriteByte(uint8(len(nickBytes)))
	buf.Write(nickBytes)

	nickname, ok := DecodeSetNickname(buf.Bytes())
	if !ok {
		t.Fatal("有效的 set-nickname 消息应解码成功")
	}
	if nickname != nick {
		t.Fatalf("nickname 不匹配: got=%s, want=%s", nickname, nick)
	}
}

func TestDecodeSetNickname_TooShort(t *testing.T) {
	_, ok := DecodeSetNickname([]byte{MsgSetNickname})
	if ok {
		t.Fatal("过短的消息应解码失败")
	}
}

func TestDecodeSetNickname_WrongType(t *testing.T) {
	_, ok := DecodeSetNickname([]byte{MsgTap, 0x03, 'a', 'b', 'c'})
	if ok {
		t.Fatal("错误的消息类型应解码失败")
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

func TestRoundTrip_SetNickname(t *testing.T) {
	nick := "Player1"
	nickBytes := []byte(nick)

	var buf bytes.Buffer
	buf.WriteByte(MsgSetNickname)
	buf.WriteByte(uint8(len(nickBytes)))
	buf.Write(nickBytes)

	nickname, ok := DecodeSetNickname(buf.Bytes())
	if !ok {
		t.Fatal("round-trip 解码应成功")
	}
	if nickname != nick {
		t.Fatalf("round-trip 值不匹配: got=%s, want=%s", nickname, nick)
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

// ─── PhaseToCode / CodeToPhase ───────────────────────────────────────

func TestPhaseToCode_RoundTrip(t *testing.T) {
	phases := []GamePhase{PhaseWaiting, PhaseCountdown, PhasePlaying, PhaseEnded}
	for _, p := range phases {
		code := PhaseToCode(p)
		got := CodeToPhase(code)
		if got != p {
			t.Errorf("CodeToPhase(PhaseToCode(%q)) = %q, want %q", p, got, p)
		}
	}
}

func TestCodeToPhase_Unknown(t *testing.T) {
	got := CodeToPhase(255)
	if got != PhaseWaiting {
		t.Fatalf("unknown code should map to PhaseWaiting, got %q", got)
	}
}

// ─── DecodeRestartVote / DecodePing ──────────────────────────────────

func TestDecodeRestartVote_Valid(t *testing.T) {
	if !DecodeRestartVote([]byte{MsgRestartVote}) {
		t.Fatal("valid restart vote should return true")
	}
}

func TestDecodeRestartVote_Invalid(t *testing.T) {
	if DecodeRestartVote([]byte{MsgTap}) {
		t.Fatal("wrong message type should return false")
	}
}

func TestDecodeRestartVote_Empty(t *testing.T) {
	if DecodeRestartVote([]byte{}) {
		t.Fatal("empty data should return false")
	}
}

func TestDecodePing_Valid(t *testing.T) {
	if !DecodePing([]byte{MsgPing}) {
		t.Fatal("valid ping should return true")
	}
}

func TestDecodePing_Invalid(t *testing.T) {
	if DecodePing([]byte{MsgTap}) {
		t.Fatal("wrong message type should return false")
	}
}

func TestDecodePing_Empty(t *testing.T) {
	if DecodePing([]byte{}) {
		t.Fatal("empty data should return false")
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

func BenchmarkDecodeSetNickname(b *testing.B) {
	nick := "TestPlayer"
	nickBytes := []byte(nick)
	var buf bytes.Buffer
	buf.WriteByte(MsgSetNickname)
	buf.WriteByte(uint8(len(nickBytes)))
	buf.Write(nickBytes)
	data := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecodeSetNickname(data)
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
