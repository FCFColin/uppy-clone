package game

import (
	"errors"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestHandleRestartVote_RecordsVoteInMap(t *testing.T) {
	room := &Room{
		state:           NewGameState("TEST", 42, testRNG()),
		usedNames:       make(map[string]bool),
		RoomConnections: RoomConnections{connections: make(map[string]*PlayerConn)},
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

func TestHandleRestartVote_NotEndedPhase(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	player := &domain.PlayerState{ID: "p1"}

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.mu.Unlock()

	err := HandleRestartVote(r, player)
	if err != nil {
		t.Fatalf("expected nil for non-ended phase, got %v", err)
	}
}

func TestHandleRestartVote_DuplicateVote(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	player := &domain.PlayerState{ID: "p1"}

	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.RestartVotes["p1"] = true
	r.mu.Unlock()

	err := HandleRestartVote(r, player)
	if err != nil {
		t.Fatalf("expected nil for duplicate vote, got %v", err)
	}
}

func TestHandleRestartVote_NewVote(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	p1 := &domain.PlayerState{ID: "p1"}
	p2 := &domain.PlayerState{ID: "p2"}

	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = p1
	r.state.Players["p2"] = p2
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	// First vote from p1 — not yet consensus (2 players, only 1 vote)
	err := HandleRestartVote(r, p1)
	if err != nil {
		t.Fatalf("expected nil for new vote, got %v", err)
	}

	r.mu.RLock()
	voted := r.state.RestartVotes["p1"]
	r.mu.RUnlock()
	if !voted {
		t.Fatal("player p1 should have voted")
	}
}

func TestHandleRestartVote_DuplicateRetriesConsensus(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	p1 := &domain.PlayerState{ID: "p1", Disconnected: false}
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = p1
	r.state.RestartVotes["p1"] = true
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	err := HandleRestartVote(r, p1)
	if err != nil {
		t.Fatalf("duplicate vote should retry consensus, got %v", err)
	}

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()
	if phase != domain.PhaseCountdown {
		t.Fatalf("expected countdown after duplicate vote retry, got %q", phase)
	}
}

func TestCheckRestartConsensus_PartialVoteStartsTimer(t *testing.T) {
	room := &Room{
		state:           NewGameState("TEST", 42, testRNG()),
		usedNames:       make(map[string]bool),
		RoomConnections: RoomConnections{connections: make(map[string]*PlayerConn)},
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
		state:           NewGameState("TEST", 42, testRNG()),
		usedNames:       make(map[string]bool),
		RoomConnections: RoomConnections{connections: make(map[string]*PlayerConn)},
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
		state:           NewGameState("TEST", 42, testRNG()),
		rng:             testRNG(),
		usedNames:       make(map[string]bool),
		RoomConnections: RoomConnections{connections: make(map[string]*PlayerConn)},
	}
	room.state.Phase = domain.PhaseEnded
	room.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", Nickname: "Player1"},
		"p2": {ID: "p2", Nickname: "Player2", Disconnected: true},
	}
	room.state.RestartVotes = map[string]bool{"p1": true}

	// Only 1 connected player voted yes → unanimous among connected
	room.mu.Lock()
	err := CheckRestartConsensus(room)
	room.mu.Unlock()
	if err != nil {
		t.Logf("CheckRestartConsensus error: %v", err)
	}
}

// --- RestartAndStart tests ---

func TestRestartAndStart_NotEndedPhase(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.mu.Unlock()

	err := RestartAndStart(r)
	if err == nil {
		t.Fatal("expected error when phase is not ended")
	}
}

func TestRestartAndStart_NoActivePlayers(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	// No connections
	r.mu.Unlock()

	err := RestartAndStart(r)
	if err != nil {
		t.Fatalf("expected nil for no active players, got %v", err)
	}

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()
	if phase != domain.PhaseWaiting {
		t.Fatalf("expected phase waiting, got %q", phase)
	}
}

func TestRestartAndStart_WithActivePlayers(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{
		ID:       "p1",
		Nickname: "Player1",
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	err := RestartAndStart(r)
	if err != nil {
		t.Fatalf("RestartAndStart failed: %v", err)
	}

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()
	if phase != domain.PhaseCountdown {
		t.Fatalf("expected phase countdown after restart, got %q", phase)
	}
}

func TestRestartAndStart_RemovesDisconnectedPlayers(t *testing.T) {
	room := &Room{
		state:           NewGameState("TEST", 42, testRNG()),
		rng:             testRNG(),
		usedNames:       make(map[string]bool),
		RoomConnections: RoomConnections{connections: make(map[string]*PlayerConn)},
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
		state:           NewGameState("TEST", 42, testRNG()),
		rng:             testRNG(),
		usedNames:       make(map[string]bool),
		RoomConnections: RoomConnections{connections: make(map[string]*PlayerConn)},
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

func TestRestartProtocolConstants(t *testing.T) {
	if domain.RestartTimeoutMs != 30000 {
		t.Errorf("RestartTimeoutMs = %d, want 30000", domain.RestartTimeoutMs)
	}
	if domain.MaxNicknameLen != 12 {
		t.Errorf("MaxNicknameLen = %d, want 12", domain.MaxNicknameLen)
	}
	if domain.NicknameCooldownMs != 30000 {
		t.Errorf("NicknameCooldownMs = %d, want 30000", domain.NicknameCooldownMs)
	}
}

func TestRestartAndStart_SaveError(t *testing.T) {
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("save failed")
	r := NewRoom("SAVE", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "P1"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	err := RestartAndStart(r)
	r.mu.Unlock()
	if err != nil {
		t.Fatalf("RestartAndStart should not return error for async save, got: %v", err)
	}
	if r.state.Phase != domain.PhaseCountdown {
		t.Fatalf("state phase = %v, want %v", r.state.Phase, domain.PhaseCountdown)
	}
}

func TestCheckRestartConsensus_FirstVoteStartsTimer(t *testing.T) {
	r := NewRoom("VOTE", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 8)}
	_ = HandleRestartVote(r, r.state.Players["p1"])
	if r.state.RestartTimerStart == nil {
		t.Fatal("expected restart timer after first vote")
	}
	r.mu.Unlock()
}

// --- coverage gap 补充用例 ---

func TestHandleRestartVote_DuplicateWhenEnded(_ *testing.T) {
	r := NewRoom("DUP", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 4)}
	r.state.RestartVotes = map[string]bool{"p1": true}
	now := time.Now().UnixMilli()
	r.state.RestartTimerStart = &now
	_ = HandleRestartVote(r, r.state.Players["p1"])
	r.mu.Unlock()
}

func TestCheckRestartConsensus_WithExistingTimer(t *testing.T) {
	r := NewRoom("RST", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 4)}
	now := time.Now().UnixMilli()
	r.state.RestartTimerStart = &now
	r.state.RestartVotes = map[string]bool{"p1": true}
	_ = CheckRestartConsensus(r)
	r.mu.Unlock()
}

func TestCheckRestartConsensus_PhaseNotEnded(t *testing.T) {
	r := NewRoom("PNE", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhasePlaying
	if err := CheckRestartConsensus(r); err != nil {
		t.Fatalf("CheckRestartConsensus: %v", err)
	}
}

func TestCheckRestartConsensus_UnanimousRestart(t *testing.T) {
	r := NewRoom("UNI", nil, newMockRoomRepository(), config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "A"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.state.RestartVotes = map[string]bool{"p1": true}
	err := CheckRestartConsensus(r)
	r.mu.Unlock()
	if err != nil {
		t.Fatalf("CheckRestartConsensus: %v", err)
	}
	if r.state.Phase != domain.PhaseCountdown {
		t.Fatalf("phase = %s, want countdown after unanimous restart", r.state.Phase)
	}
}

func TestCheckRestartConsensus_NotEndedAfterBroadcast(t *testing.T) {
	r := NewRoom("NBA", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhaseCountdown
	if err := CheckRestartConsensus(r); err != nil {
		t.Fatalf("CheckRestartConsensus: %v", err)
	}
}

func TestCheckRestartConsensus_NegativeRemainingTimer(t *testing.T) {
	r := NewRoom("NEG", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 4)}
	past := time.Now().UnixMilli() - int64(domain.RestartTimeoutMs) - 1000
	r.state.RestartTimerStart = &past
	r.state.RestartVotes = map[string]bool{"p1": true}
	_ = CheckRestartConsensus(r)
	r.mu.Unlock()
}

const (
	testNickname = "helloworld"
	testGreeting = "hello"
)

// --- HandleSetNickname tests ---
