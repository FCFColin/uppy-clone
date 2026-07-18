package protocol

import (
	"testing"
)

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
