package game

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
)

func TestHandleSetNickname(t *testing.T) {
	now := time.Now().UnixMilli()

	cases := []struct {
		name             string
		input            string
		lastChange       int64
		usedNames        map[string]bool
		wantAccept       bool
		wantNicknameIs   string // if non-empty, player.Nickname must equal this
		wantNicknameNot  string // if non-empty, player.Nickname must not equal this
		checkUsedNewName bool   // verify usedNames["NewName"] is true and usedNames["OldName"] is false
		checkTimestamp   bool   // verify LastNicknameChange updated to ~now
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
			name:            "HTMLCharacters",
			input:           "test<script>",
			usedNames:       map[string]bool{"OldName": true},
			wantAccept:      true,
			wantNicknameNot: "test<script>",
		},
		{
			name:       "SameNickname",
			input:      "SameName",
			usedNames:  map[string]bool{"SameName": true},
			wantAccept: false,
		},
		{
			name:            "DuplicateNickname",
			input:           "TakenName",
			usedNames:       map[string]bool{"OldName": true, "TakenName": true},
			wantAccept:      true,
			wantNicknameNot: "TakenName",
		},
		{
			name:       "EmptyNickname",
			input:      "",
			usedNames:  map[string]bool{"OldName": true},
			wantAccept: false,
		},
		{
			name:       "CooldownNotExpired",
			input:      "NewName",
			lastChange: now - 1000,
			usedNames:  map[string]bool{"OldName": true},
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
			name:            "DangerousChars",
			input:           "<script>alert(1)</script>",
			usedNames:       map[string]bool{"OldName": true},
			wantAccept:      true,
			wantNicknameNot: "<script>alert(1)</script>",
		},
		{
			name:            "LengthLimit",
			input:           "abcdefghijklmnop",
			usedNames:       map[string]bool{"OldName": true},
			wantAccept:      true,
			wantNicknameNot: "abcdefghijklmnop",
		},
		{
			name:       "TruncateLongName",
			input:      "这是一个非常长的名字用来测试截断功能",
			usedNames:  map[string]bool{"OldName": true},
			wantAccept: true,
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
			if c.name == "LengthLimit" || c.name == "TruncateLongName" {
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
