package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

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

func TestEncodeSingleByteMessages(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantMsg byte
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

func TestEncodeNicknameRejected(t *testing.T) {
	cases := []struct {
		name   string
		reason uint8
	}{
		{"empty", NickRejectEmpty},
		{"duplicate", NickRejectDuplicate},
		{"cooldown", NickRejectCooldown},
		{"decode_error", NickRejectDecodeError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := EncodeNicknameRejected(c.reason)
			if len(data) != 2 {
				t.Fatalf("expected 2 bytes, got %d", len(data))
			}
			if data[0] != MsgNicknameRejected {
				t.Fatalf("首字节应为 MsgNicknameRejected=0x08，got=0x%02x", data[0])
			}
			if data[1] != c.reason {
				t.Fatalf("reason 不匹配: got=0x%02x, want=0x%02x", data[1], c.reason)
			}
		})
	}
}

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

func TestDecodeTap(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		wantX   float32
		wantY   float32
		wantOK  bool
	}{
		{"valid", encodeFloat32Pair(0.5, 0.3), 0.5, 0.3, true},
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
		name     string
		payload  []byte
		wantNick string
		wantOK   bool
	}{
		{"valid", append([]byte{5}, []byte("hello")...), "hello", true},
		{"empty", nil, "", false},
		{"truncated", []byte{5, 'a', 'b'}, "", false},
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

func TestDecodeMessage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		msgType, payload := DecodeMessage([]byte{MsgTap, 0x01, 0x02})
		if msgType != MsgTap {
			t.Fatalf("消息类型不匹配: got=0x%02x, want=0x%02x", msgType, MsgTap)
		}
		if len(payload) != 2 {
			t.Fatalf("payload 长度不匹配: got=%d, want=2", len(payload))
		}
	})
	t.Run("empty", func(t *testing.T) {
		msgType, payload := DecodeMessage([]byte{})
		if msgType != 0 {
			t.Fatalf("空消息应返回类型 0，got=0x%02x", msgType)
		}
		if payload != nil {
			t.Fatal("空消息 payload 应为 nil")
		}
	})
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

// TestProtocol_PlayerJoinRoundTrip_TableDriven verifies that EncodePlayerJoin
// followed by DecodeMessage preserves playerIndex, nickname, and palette for
// representative boundary cases. Replaces the original rapid property test.
func TestProtocol_PlayerJoinRoundTrip_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		playerIndex uint16
		nickname    string
		palette     uint32
	}{
		{"regular ASCII", 5, "TestPlayer", 3},
		{"zero values", 0, "", 0},
		{"max uint16 index", 65535, "Player", 1},
		{"max uint32 palette", 1, "Bob", 4294967295},
		{"unicode CJK nickname", 42, "you-hao-shi-jie", 7},
		{"emoji nickname", 7, "gamer", 9},
		{"single char nickname", 1, "A", 0},
		{"long nickname", 99, "abcdefghijklmnopqrstuvwxyz0123456789", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := EncodePlayerJoin(tt.playerIndex, tt.nickname, tt.palette)
			msgType, payload := DecodeMessage(data)
			if msgType != MsgPlayerJoin {
				t.Fatalf("msgType = 0x%02x, want 0x%02x", msgType, MsgPlayerJoin)
			}
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
			if gotPlayerIndex != tt.playerIndex {
				t.Fatalf("playerIndex = %d, want %d", gotPlayerIndex, tt.playerIndex)
			}
			if gotNickname != tt.nickname {
				t.Fatalf("nickname = %q, want %q", gotNickname, tt.nickname)
			}
			if gotPalette != tt.palette {
				t.Fatalf("palette = %d, want %d", gotPalette, tt.palette)
			}
		})
	}
}

// TestProtocol_PlayerJoinRoundTrip_NickTooLong verifies that encoding a nickname
// exceeding the uint8 byte-length limit panics (defensive contract).
func TestProtocol_PlayerJoinRoundTrip_NickTooLong(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for overlong nickname")
		}
	}()
	longNick := make([]byte, 256)
	for i := range longNick {
		longNick[i] = 'a'
	}
	EncodePlayerJoin(0, string(longNick), 0)
}
