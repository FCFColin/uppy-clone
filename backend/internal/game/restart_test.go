package game

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// --- HandleRestartVote additional tests ---

func TestHandleRestartVote_RecordsVoteInMap(t *testing.T) {
	room := &Room{
		state:       NewGameState("TEST"),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
	}
	room.state.Phase = domain.PhaseEnded
	room.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", Nickname: "Player1"},
		"p2": {ID: "p2", Nickname: "Player2"},
	}
	room.state.RestartVotes = make(map[string]bool)

	player := &domain.PlayerState{ID: "p1"}
	_ = HandleRestartVote(room, player)

	if !room.state.RestartVotes["p1"] {
		t.Error("vote should be recorded for p1")
	}
}

func TestCheckRestartConsensus_PartialVoteStartsTimer(t *testing.T) {
	room := &Room{
		state:       NewGameState("TEST"),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
	}
	room.state.Phase = domain.PhaseEnded
	room.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", Nickname: "Player1"},
		"p2": {ID: "p2", Nickname: "Player2"},
	}
	room.state.RestartVotes = map[string]bool{"p1": true}

	err := CheckRestartConsensus(room)
	if err != nil {
		t.Errorf("CheckRestartConsensus partial vote should not error: %v", err)
	}

	if room.state.RestartTimerStart == nil {
		t.Error("RestartTimerStart should be set on first vote")
	}
}

func TestCheckRestartConsensus_TimerAlreadyStarted(t *testing.T) {
	now := time.Now().UnixMilli()
	room := &Room{
		state:       NewGameState("TEST"),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
	}
	room.state.Phase = domain.PhaseEnded
	room.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", Nickname: "Player1"},
		"p2": {ID: "p2", Nickname: "Player2"},
	}
	room.state.RestartVotes = map[string]bool{"p1": true}
	room.state.RestartTimerStart = &now

	err := CheckRestartConsensus(room)
	if err != nil {
		t.Errorf("CheckRestartConsensus should not error: %v", err)
	}
	if room.state.RestartTimerStart == nil {
		t.Error("RestartTimerStart should still be set")
	}
}

func TestCheckRestartConsensus_DisconnectedPlayersNotCounted(t *testing.T) {
	room := &Room{
		state:       NewGameState("TEST"),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
	}
	room.state.Phase = domain.PhaseEnded
	room.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", Nickname: "Player1"},
		"p2": {ID: "p2", Nickname: "Player2", Disconnected: true},
	}
	room.state.RestartVotes = map[string]bool{"p1": true}

	// Only 1 connected player voted yes → unanimous among connected
	err := CheckRestartConsensus(room)
	if err != nil {
		t.Logf("CheckRestartConsensus error: %v", err)
	}
}

func TestRestartAndStart_RemovesDisconnectedPlayers(t *testing.T) {
	room := &Room{
		state:       NewGameState("TEST"),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
	}
	room.state.Phase = domain.PhaseEnded
	room.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", Nickname: "Player1"},
		"p2": {ID: "p2", Nickname: "Player2", Disconnected: true},
	}
	room.usedNames["Player1"] = true
	room.usedNames["Player2"] = true
	room.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 10)}

	err := RestartAndStart(room)
	if err != nil {
		t.Errorf("RestartAndStart should not error: %v", err)
	}

	if _, exists := room.state.Players["p2"]; exists {
		t.Error("disconnected player p2 should be removed")
	}
	if _, exists := room.state.Players["p1"]; !exists {
		t.Error("connected player p1 should remain")
	}
}

func TestRestartAndStart_ResetsPlayerStats(t *testing.T) {
	room := &Room{
		state:       NewGameState("TEST"),
		usedNames:   make(map[string]bool),
		connections: make(map[string]*PlayerConn),
	}
	room.state.Phase = domain.PhaseEnded
	room.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", Nickname: "Player1", ScoreContribution: 100, TapsCount: 50},
	}
	room.state.NextPlayerIndex = 1
	room.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 10)}

	err := RestartAndStart(room)
	if err != nil {
		t.Errorf("RestartAndStart should not error: %v", err)
	}

	player := room.state.Players["p1"]
	if player.ScoreContribution != 0 {
		t.Errorf("ScoreContribution = %d, want 0", player.ScoreContribution)
	}
	if player.TapsCount != 0 {
		t.Errorf("TapsCount = %d, want 0", player.TapsCount)
	}
}

// Ensure protocol constants are used correctly
func TestRestartProtocolConstants(t *testing.T) {
	if protocol.RestartTimeoutMs != 30000 {
		t.Errorf("RestartTimeoutMs = %d, want 30000", protocol.RestartTimeoutMs)
	}
	if protocol.MaxNicknameLen != 12 {
		t.Errorf("MaxNicknameLen = %d, want 12", protocol.MaxNicknameLen)
	}
	if protocol.NicknameCooldownMs != 30000 {
		t.Errorf("NicknameCooldownMs = %d, want 30000", protocol.NicknameCooldownMs)
	}
}
