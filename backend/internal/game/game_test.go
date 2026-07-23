package game

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
)

func TestGenerateUniqueNickname_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		usedNames      map[string]bool
		wantExact      string
		wantNotEqual   string
		wantNotInUsed  bool
		wantNonEmpty   bool
		wantNotContain []string
		wantMaxRunes   int
	}{
		{name: "NoConflict", input: "玩家甲", usedNames: map[string]bool{"已占用": true}, wantExact: "玩家甲"},
		{name: "Conflict", input: "玩家甲", usedNames: map[string]bool{"玩家甲": true}, wantNotEqual: "玩家甲", wantNotInUsed: true},
		{name: "Empty", input: "", usedNames: map[string]bool{}, wantNonEmpty: true},
		{name: "DangerousChars", input: "<script>alert(1)</script>", usedNames: map[string]bool{}, wantNotContain: []string{"<", ">"}},
		{name: "Truncation", input: strings.Repeat("a", 50), usedNames: map[string]bool{}, wantMaxRunes: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateUniqueNickname(tt.input, tt.usedNames)
			if tt.wantExact != "" && result != tt.wantExact {
				t.Fatalf("不重复的名字应直接使用，got=%s", result)
			}
			if tt.wantNotEqual != "" && result == tt.wantNotEqual {
				t.Fatalf("重复的名字应生成随机名字, got=%s", result)
			}
			if tt.wantNotInUsed && tt.usedNames[result] {
				t.Fatalf("生成的名字不应在已用列表中, got=%s", result)
			}
			if tt.wantNonEmpty && len(result) == 0 {
				t.Fatal("空名字应生成随机昵称")
			}
			for _, c := range tt.wantNotContain {
				if strings.Contains(result, c) {
					t.Fatalf("危险字符名字应被拒绝并生成随机昵称，got=%s", result)
				}
			}
			if tt.wantMaxRunes > 0 && len([]rune(result)) > tt.wantMaxRunes {
				t.Fatalf("long client name should be truncated to %d chars, got %d", tt.wantMaxRunes, len([]rune(result)))
			}
		})
	}
}

func TestHandleSetNickname(t *testing.T) {
	now := time.Now().UnixMilli()

	cases := []struct {
		name             string
		input            string
		lastChange       int64
		usedNames        map[string]bool
		wantAccept       bool
		wantNicknameIs   string
		wantNicknameNot  string
		checkUsedNewName bool
		checkTimestamp   bool
	}{
		{
			name:             "FirstChangeSkipsCooldown",
			input:            "NewName",
			usedNames:        map[string]bool{"OldName": true},
			wantAccept:       true,
			checkUsedNewName: true,
			checkTimestamp:   true,
		},
		{
			name:           "ControlCharacters",
			input:          "hello\x00world",
			usedNames:      map[string]bool{"OldName": true},
			wantAccept:     true,
			wantNicknameIs: testNickname,
		},
		{
			name:       "SameNickname",
			input:      "SameName",
			usedNames:  map[string]bool{"SameName": true},
			wantAccept: false,
		},
		{
			name:       "CooldownExpired",
			input:      "NewName",
			lastChange: now - domain.NicknameCooldownMs - 1000,
			usedNames:  map[string]bool{"OldName": true},
			wantAccept: true,
		},
		{
			name:            "LengthLimit",
			input:           "abcdefghijklmnop",
			usedNames:       map[string]bool{"OldName": true},
			wantAccept:      true,
			wantNicknameNot: "abcdefghijklmnop",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := NewGameState("TEST", 42, testRNG())
			oldName := "OldName"
			if c.input == "SameName" {
				oldName = "SameName"
			}
			player := &domain.PlayerState{
				ID:                 "p1",
				Nickname:           oldName,
				LastNicknameChange: c.lastChange,
			}
			state.Players = map[string]*domain.PlayerState{"p1": player}

			before := time.Now().UnixMilli()
			result := HandleSetNickname(state, player, c.input, c.usedNames)
			after := time.Now().UnixMilli()

			if result != c.wantAccept {
				t.Fatalf("HandleSetNickname result = %v, want %v", result, c.wantAccept)
			}
			if c.wantNicknameIs != "" && player.Nickname != c.wantNicknameIs {
				t.Errorf("nickname = %q, want %q", player.Nickname, c.wantNicknameIs)
			}
			if c.wantNicknameNot != "" && player.Nickname == c.wantNicknameNot {
				t.Errorf("nickname should not equal %q", c.wantNicknameNot)
			}
			if c.name == "LengthLimit" {
				if got := len([]rune(player.Nickname)); got > domain.MaxNicknameLen {
					t.Errorf("nickname length = %d, want <= %d", got, domain.MaxNicknameLen)
				}
			}
			if c.checkUsedNewName {
				if !c.usedNames["NewName"] {
					t.Error("NewName should be in usedNames")
				}
				if c.usedNames["OldName"] {
					t.Error("OldName should be removed from usedNames")
				}
			}
			if c.checkTimestamp {
				if player.LastNicknameChange < before || player.LastNicknameChange > after {
					t.Errorf("LastNicknameChange = %d, expected between %d and %d", player.LastNicknameChange, before, after)
				}
			}
		})
	}
}

func TestValidateTapRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()
	room := &Room{state: NewGameState("TEST", 42, testRNG())}

	t.Run("rejects when not playing", func(t *testing.T) {
		room.state.Phase = domain.PhaseWaiting
		player := &domain.PlayerState{CooldownEndTime: 0}
		if room.validateTapRequest(player, now) {
			t.Error("validateTapRequest should reject non-playing phase")
		}
	})

	t.Run("rejects when on cooldown", func(t *testing.T) {
		room.state.Phase = domain.PhasePlaying
		player := &domain.PlayerState{CooldownEndTime: now + 1000}
		if room.validateTapRequest(player, now) {
			t.Error("validateTapRequest should reject when on cooldown")
		}
	})

	t.Run("accepts valid tap", func(t *testing.T) {
		room.state.Phase = domain.PhasePlaying
		player := &domain.PlayerState{CooldownEndTime: 0}
		if !room.validateTapRequest(player, now) {
			t.Error("validateTapRequest should accept valid tap")
		}
	})

	t.Run("accepts expired cooldown", func(t *testing.T) {
		room.state.Phase = domain.PhasePlaying
		player := &domain.PlayerState{CooldownEndTime: now - 1}
		if !room.validateTapRequest(player, now) {
			t.Error("validateTapRequest should accept expired cooldown")
		}
	})
}

func TestDecodeTapPayload(t *testing.T) {
	t.Parallel()

	room := &Room{state: NewGameState("TEST", 42, testRNG())}

	t.Run("rejects short payload", func(t *testing.T) {
		_, _, ok := room.decodeTapPayload([]byte{0, 1, 2})
		if ok {
			t.Error("decodeTapPayload should reject < 8 bytes")
		}
	})

	invalidCoords := []struct {
		name string
		x, y float32
	}{
		{"NaN", float32(math.NaN()), 0.5},
		{"OutOfRange", 1.5, 0.5},
	}
	for _, c := range invalidCoords {
		t.Run("rejects "+c.name, func(t *testing.T) {
			_, _, ok := room.decodeTapPayload(encodeTapTestPayload(c.x, c.y))
			if ok {
				t.Errorf("decodeTapPayload should reject %s coordinates", c.name)
			}
		})
	}

	t.Run("accepts valid coordinates", func(t *testing.T) {
		payload := encodeTapTestPayload(0.5, 0.3)
		x, y, ok := room.decodeTapPayload(payload)
		if !ok || x != 0.5 || y != 0.3 {
			t.Errorf("decodeTapPayload = (%v, %v, %v), want (0.5, 0.3, true)", x, y, ok)
		}
	})

	t.Run("accepts boundary values", func(t *testing.T) {
		payload := encodeTapTestPayload(0, 1)
		x, y, ok := room.decodeTapPayload(payload)
		if !ok || x != 0 || y != 1 {
			t.Errorf("decodeTapPayload = (%v, %v, %v), want (0, 1, true)", x, y, ok)
		}
	})
}
