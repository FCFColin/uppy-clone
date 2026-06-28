package game

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

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

func TestHandleSetNickname_EmptyNickname(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "", usedNames)

	if result {
		t.Fatal("empty nickname should return false")
	}
}

func TestHandleSetNickname_CooldownNotExpired(t *testing.T) {
	state := NewGameState("TEST")
	now := time.Now().UnixMilli()
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: now - 1000,
	}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "NewName", usedNames)

	if result {
		t.Fatal("nickname change during cooldown should return false")
	}
}

func TestHandleSetNickname_CooldownExpired(t *testing.T) {
	state := NewGameState("TEST")
	now := time.Now().UnixMilli()
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: now - protocol.NicknameCooldownMs - 1000,
	}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "NewName", usedNames)

	if !result {
		t.Fatal("nickname change after cooldown should succeed")
	}
}

func TestHandleSetNickname_DangerousChars(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "<script>alert(1)</script>", usedNames)

	if !result {
		t.Fatal("dangerous chars should be sanitized and change should succeed")
	}
	if player.Nickname == "<script>alert(1)</script>" {
		t.Fatal("dangerous chars should be stripped from nickname")
	}
}

func TestHandleSetNickname_TruncateLongName(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	usedNames := map[string]bool{"OldName": true}

	longName := "这是一个非常长的名字用来测试截断功能"
	result := HandleSetNickname(state, player, longName, usedNames)

	if !result {
		t.Fatal("long nickname should succeed (truncated)")
	}
	if len([]rune(player.Nickname)) > protocol.MaxNicknameLen {
		t.Fatalf("nickname should be truncated to %d chars, got %d", protocol.MaxNicknameLen, len([]rune(player.Nickname)))
	}
}
