package game

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

import (
	"strings"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

const (
	testNickname = "helloworld"
	testGreeting = "hello"
)

// --- HandleSetNickname additional tests ---

func TestHandleSetNickname_FirstChangeSkipsCooldown(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "NewName", usedNames)
	if !result {
		t.Error("HandleSetNickname should allow first change regardless of cooldown")
	}
}

func TestHandleSetNickname_ControlCharacters(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "hello\x00world", usedNames)
	if !result {
		t.Error("HandleSetNickname should sanitize and accept")
	}
	if player.Nickname != testNickname {
		t.Errorf("nickname = %q, want %q", player.Nickname, testNickname)
	}
}

func TestHandleSetNickname_HTMLCharacters(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "test<script>", usedNames)
	if !result {
		t.Error("HandleSetNickname should sanitize and accept")
	}
	if player.Nickname == "test<script>" {
		t.Error("HTML characters should be removed from nickname")
	}
}

func TestHandleSetNickname_LengthLimit(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"OldName": true}

	longName := "abcdefghijklmnop"
	result := HandleSetNickname(state, player, longName, usedNames)
	if !result {
		t.Error("HandleSetNickname should accept and truncate long nickname")
	}
	if len([]rune(player.Nickname)) > protocol.MaxNicknameLen {
		t.Errorf("nickname length = %d, want <= %d", len([]rune(player.Nickname)), protocol.MaxNicknameLen)
	}
}

func TestHandleSetNickname_SameNickname(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "SameName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"SameName": true}

	result := HandleSetNickname(state, player, "SameName", usedNames)
	if result {
		t.Error("HandleSetNickname should return false when nickname is the same")
	}
}

func TestHandleSetNickname_DuplicateNickname(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"OldName": true, "TakenName": true}

	result := HandleSetNickname(state, player, "TakenName", usedNames)
	if !result {
		t.Error("HandleSetNickname should generate unique name for duplicate")
	}
	if player.Nickname == "TakenName" {
		t.Error("HandleSetNickname should not allow duplicate nickname")
	}
}

func TestHandleSetNickname_UpdatesUsedNames(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"OldName": true}

	HandleSetNickname(state, player, "NewName", usedNames)

	if !usedNames["NewName"] {
		t.Error("NewName should be in usedNames")
	}
	if usedNames["OldName"] {
		t.Error("OldName should be removed from usedNames")
	}
}

func TestHandleSetNickname_UpdatesLastNicknameChange(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	state.Players = map[string]*domain.PlayerState{"p1": player}
	usedNames := map[string]bool{"OldName": true}

	before := time.Now().UnixMilli()
	HandleSetNickname(state, player, "NewName", usedNames)
	after := time.Now().UnixMilli()

	if player.LastNicknameChange < before || player.LastNicknameChange > after {
		t.Errorf("LastNicknameChange = %d, expected between %d and %d", player.LastNicknameChange, before, after)
	}
}

// --- sanitizeNickname tests ---

func TestSanitizeNickname_ControlChars(t *testing.T) {
	result := sanitizeNickname("hello\x00world\x01test")
	// validate.Nickname truncates to protocol.MaxNicknameLen (12 runes).
	want := "helloworldte"
	if result != want {
		t.Errorf("sanitizeNickname = %q, want %q", result, want)
	}
}

func TestSanitizeNickname_ZeroWidthChars(t *testing.T) {
	result := sanitizeNickname("hello\u200Bworld")
	if result != testNickname {
		t.Errorf("sanitizeNickname = %q, want %q", result, testNickname)
	}
}

func TestSanitizeNickname_HTMLChars(t *testing.T) {
	result := sanitizeNickname("test<script>alert('xss')</script>")
	if strings.ContainsAny(result, "<>'\"`&") {
		t.Errorf("sanitizeNickname should remove HTML chars, got %q", result)
	}
}

func TestSanitizeNickname_TrimSpace(t *testing.T) {
	result := sanitizeNickname("  hello  ")
	if result != testGreeting {
		t.Errorf("sanitizeNickname = %q, want %q", result, testGreeting)
	}
}

func TestSanitizeNickname_EmptyAfterSanitization(t *testing.T) {
	result := sanitizeNickname("\x00\x01\x02")
	if result != "" {
		t.Errorf("sanitizeNickname = %q, want empty string", result)
	}
}
