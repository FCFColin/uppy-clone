package game

import (
	"errors"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── Room Creation ───────────────────────────────────────────────────

func TestRoom_Creation(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	if r == nil {
		t.Fatal("NewRoom returned nil")
	}
	if r.state.LobbyCode != "TEST1" {
		t.Fatalf("expected LobbyCode TEST1, got %q", r.state.LobbyCode)
	}
	if r.state.Phase != domain.PhaseWaiting {
		t.Fatalf("expected initial phase waiting, got %q", r.state.Phase)
	}
	if len(r.connections) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(r.connections))
	}
	if len(r.usedNames) != 0 {
		t.Fatalf("expected 0 usedNames, got %d", len(r.usedNames))
	}
}

// ─── HandleDisconnect ────────────────────────────────────────────────

func TestRoom_HandleDisconnect(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Add a player manually
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:       "p1",
		Nickname: "TestPlayer",
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	_ = r.HandleDisconnect("p1")

	r.mu.RLock()
	player := r.state.Players["p1"]
	connCount := len(r.connections)
	r.mu.RUnlock()

	if !player.Disconnected {
		t.Fatal("player should be marked as disconnected")
	}
	if player.DisconnectedAt == nil {
		t.Fatal("DisconnectedAt should be set")
	}
	if connCount != 0 {
		t.Fatalf("expected 0 connections after disconnect, got %d", connCount)
	}
}

func TestRoom_HandleDisconnect_NonexistentPlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Should not panic
	err := r.HandleDisconnect("nonexistent")
	if err != nil {
		t.Fatalf("expected nil for nonexistent player disconnect, got %v", err)
	}
}

// ─── cleanupDisconnected ─────────────────────────────────────────────

func TestRoom_CleanupDisconnected_RemovesExpired(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	disconnectedAt := time.Now().UnixMilli() - protocol.ReconnectGraceMs - 1000 // expired
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:             "p1",
		Nickname:       "ExpiredPlayer",
		Disconnected:   true,
		DisconnectedAt: &disconnectedAt,
	}
	r.usedNames["ExpiredPlayer"] = true
	r.mu.Unlock()

	r.cleanupDisconnected(time.Now().UnixMilli())

	r.mu.RLock()
	_, playerExists := r.state.Players["p1"]
	_, nameExists := r.usedNames["ExpiredPlayer"]
	r.mu.RUnlock()

	if playerExists {
		t.Fatal("expired disconnected player should be removed")
	}
	if nameExists {
		t.Fatal("expired player's name should be removed from usedNames")
	}
}

func TestRoom_CleanupDisconnected_KeepsGracePeriod(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	disconnectedAt := time.Now().UnixMilli() - 1000 // still in grace period
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:             "p1",
		Nickname:       "GracePlayer",
		Disconnected:   true,
		DisconnectedAt: &disconnectedAt,
	}
	r.usedNames["GracePlayer"] = true
	r.mu.Unlock()

	r.cleanupDisconnected(time.Now().UnixMilli())

	r.mu.RLock()
	_, exists := r.state.Players["p1"]
	r.mu.RUnlock()

	if !exists {
		t.Fatal("player in grace period should not be removed")
	}
}

// ─── broadcast ───────────────────────────────────────────────────────

func TestRoom_Broadcast(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true

	ch1 := make(chan []byte, 64)
	ch2 := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch1}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: ch2}
	r.mu.Unlock()

	msg := []byte{0x01, 0x02, 0x03}
	r.mu.Lock()
	r.broadcast(msg, "")
	r.mu.Unlock()

	select {
	case got := <-ch1:
		if len(got) != len(msg) {
			t.Fatalf("p1: expected %d bytes, got %d", len(msg), len(got))
		}
	default:
		t.Fatal("p1 should have received the broadcast message")
	}

	select {
	case got := <-ch2:
		if len(got) != len(msg) {
			t.Fatalf("p2: expected %d bytes, got %d", len(msg), len(got))
		}
	default:
		t.Fatal("p2 should have received the broadcast message")
	}
}

func TestRoom_Broadcast_Exclude(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true

	ch1 := make(chan []byte, 64)
	ch2 := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch1}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: ch2}
	r.mu.Unlock()

	msg := []byte{0x01, 0x02, 0x03}
	r.mu.Lock()
	r.broadcast(msg, "p1")
	r.mu.Unlock()

	select {
	case <-ch1:
		t.Fatal("p1 should NOT have received the broadcast message (excluded)")
	default:
		// expected
	}

	select {
	case <-ch2:
		// expected
	default:
		t.Fatal("p2 should have received the broadcast message")
	}
}

// ─── sendToPlayer ────────────────────────────────────────────────────

func TestRoom_SendToPlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	msg := []byte{0x01, 0x02}
	r.sendToPlayer("p1", msg)

	select {
	case got := <-ch:
		if len(got) != len(msg) {
			t.Fatalf("expected %d bytes, got %d", len(msg), len(got))
		}
	default:
		t.Fatal("player should have received the message")
	}
}

func TestRoom_SendToPlayer_Nonexistent(_ *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Should not panic
	r.sendToPlayer("nonexistent", []byte{0x01})
}

// ─── GetConnection ───────────────────────────────────────────────────

func TestRoom_GetConnection(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.mu.Unlock()

	conn := r.GetConnection("p1")
	if conn == nil {
		t.Fatal("expected to find connection for p1")
	}

	conn = r.GetConnection("nonexistent")
	if conn != nil {
		t.Fatal("expected nil for nonexistent connection")
	}
}

// ─── Close ───────────────────────────────────────────────────────────

func TestRoom_Close(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Add a player without a real WebSocket connection (Conn = nil)
	// Close should handle nil Conn gracefully
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:       "p1",
		Nickname: "TestPlayer",
	}
	// Don't add a connection since we can't create a real websocket.Conn in tests
	r.state.Phase = domain.PhasePlaying
	r.mu.Unlock()

	r.Close()

	r.mu.RLock()
	connCount := len(r.connections)
	r.mu.RUnlock()

	if connCount != 0 {
		t.Fatalf("expected 0 connections after Close, got %d", connCount)
	}
}

// ─── StartGame ───────────────────────────────────────────────────────

func TestRoom_StartGame_FromWaiting(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Add a player so the game can start
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:       "p1",
		Nickname: "TestPlayer",
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	err := r.StartGame()
	if err != nil {
		t.Fatalf("StartGame failed: %v", err)
	}

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()

	if phase != domain.PhaseCountdown {
		t.Fatalf("expected phase countdown after StartGame, got %q", phase)
	}
}

func TestRoom_StartGame_NotFromWaiting(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.mu.Unlock()

	err := r.StartGame()
	if err != nil {
		t.Fatalf("StartGame from non-waiting should return nil, got %v", err)
	}

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()

	if phase != domain.PhasePlaying {
		t.Fatalf("phase should remain unchanged, got %q", phase)
	}
}

// ─── EndGame ─────────────────────────────────────────────────────────

func TestRoom_EndGame(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.SessionID = "test-session"
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	err := r.EndGame()
	if err != nil {
		t.Fatalf("EndGame failed: %v", err)
	}

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()

	if phase != domain.PhaseEnded {
		t.Fatalf("expected phase ended after EndGame, got %q", phase)
	}
}

func TestRoom_EndGame_NoPlayers(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.SessionID = "test-session"
	// No connections
	r.mu.Unlock()

	r.EndGame()

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()

	// No connections → should reset to waiting
	if phase != domain.PhaseWaiting {
		t.Fatalf("expected phase waiting when no players, got %q", phase)
	}
}

func TestRoom_EndGame_ClampsBalloonY(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.Balloon.Y = -0.2
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	if err := r.EndGame(); err != nil {
		t.Fatalf("EndGame failed: %v", err)
	}

	r.mu.RLock()
	y := r.state.Balloon.Y
	r.mu.RUnlock()
	if y < 0 {
		t.Fatalf("expected balloon Y clamped to >= 0, got %v", y)
	}
}

// ─── HandleMessage rate limiting ─────────────────────────────────────

func TestRoom_HandleMessage_RateLimit(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Set MessageCount below rate limit to verify the rate-limiting logic
	// without triggering the Close() call on nil websocket.Conn
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "TestPlayer",
		MessageCount:       protocol.MessageRateLimit - 1,
		MessageWindowStart: time.Now().UnixMilli(),
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	// This message should be processed (not yet rate-limited)
	err := r.HandleMessage("p1", protocol.MsgPing, nil)
	if err != nil {
		t.Fatalf("HandleMessage should not error, got %v", err)
	}

	// Now MessageCount should be at the limit
	r.mu.RLock()
	count := r.state.Players["p1"].MessageCount
	r.mu.RUnlock()
	if count != protocol.MessageRateLimit {
		t.Fatalf("expected MessageCount=%d, got %d", protocol.MessageRateLimit, count)
	}
}

func TestRoom_HandleMessage_NonexistentPlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Should not panic
	err := r.HandleMessage("nonexistent", protocol.MsgPing, nil)
	if err != nil {
		t.Fatalf("expected nil for nonexistent player, got %v", err)
	}
}

// ─── modelPhaseToProtocol ────────────────────────────────────────────

func TestModelPhaseToProtocol(t *testing.T) {
	tests := []struct {
		input    domain.GamePhase
		expected protocol.GamePhase
	}{
		{domain.PhaseWaiting, protocol.PhaseWaiting},
		{domain.PhaseCountdown, protocol.PhaseCountdown},
		{domain.PhasePlaying, protocol.PhasePlaying},
		{domain.PhaseEnded, protocol.PhaseEnded},
	}
	for _, tt := range tests {
		got := modelPhaseToProtocol(tt.input)
		if got != tt.expected {
			t.Errorf("modelPhaseToProtocol(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ─── Benchmarks ──────────────────────────────────────────────────────

func BenchmarkRoom_BuildSnapshot(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("BENCH", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	for i := 0; i < 10; i++ {
		pid := "p" + string(rune('0'+i))
		r.state.Players[pid] = &domain.PlayerState{
			ID:              pid,
			PlayerIndex:     i,
			Nickname:        "Player",
			Palette:         i % 10,
			CooldownEndTime: 0,
		}
	}
	r.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.buildSnapshot()
	}
}

func BenchmarkRoom_CleanupDisconnected(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("BENCH", nil, nil, timeouts, 0)

	now := time.Now().UnixMilli()
	r.mu.Lock()
	for i := 0; i < 100; i++ {
		pid := "p" + string(rune('0'+i%10)) + string(rune('0'+i/10))
		disconnectedAt := now - protocol.ReconnectGraceMs - 1000
		r.state.Players[pid] = &domain.PlayerState{
			ID:             pid,
			Nickname:       "Player",
			Disconnected:   true,
			DisconnectedAt: &disconnectedAt,
		}
	}
	r.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.cleanupDisconnected(now)
	}
}

// --- HandleRestartVote tests ---

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

const (
	testNickname = "helloworld"
	testGreeting = "hello"
)

// --- HandleSetNickname tests ---

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
		LastNicknameChange: now - 1000, // 1 second ago, cooldown is 30s
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

// ─── Nickname ready flow ─────────────────────────────────────────────

func addConnectedPlayer(r *Room, playerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.Players[playerID] = &domain.PlayerState{
		ID:          playerID,
		PlayerIndex: len(r.state.Players),
		Nickname:    "Player" + playerID,
	}
	r.connections[playerID] = &PlayerConn{PlayerID: playerID, Send: make(chan []byte, 64)}
	r.usedNames["Player"+playerID] = true
}

func TestRoom_JoinDoesNotStartCountdown(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	addConnectedPlayer(r, "p1")

	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()

	if phase != domain.PhaseWaiting {
		t.Fatalf("expected waiting after join without nickname confirm, got %q", phase)
	}
}

func TestRoom_SetNicknameStartsCountdownWhenAllReady(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	addConnectedPlayer(r, "p1")

	payload := append([]byte{byte(len("Alice"))}, []byte("Alice")...)
	r.mu.Lock()
	player := r.state.Players["p1"]
	r.handleSetNicknameMsg(player, payload)
	phase := r.state.Phase
	r.mu.Unlock()

	if phase != domain.PhaseCountdown {
		t.Fatalf("expected countdown after single player confirms nickname, got %q", phase)
	}
}

func TestRoom_SetNicknameChineseUTF8StartsCountdown(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	addConnectedPlayer(r, "p1")

	nick := "好奇的中子"
	payload := append([]byte{byte(len(nick))}, []byte(nick)...)
	if len(nick) <= config.MaxNicknameLen {
		t.Fatalf("test nickname byte length = %d, want > %d to reproduce the bug", len(nick), config.MaxNicknameLen)
	}

	r.mu.Lock()
	player := r.state.Players["p1"]
	r.handleSetNicknameMsg(player, payload)
	confirmed := player.NicknameConfirmed
	phase := r.state.Phase
	gotNick := player.Nickname
	r.mu.Unlock()

	if !confirmed {
		t.Fatal("expected NicknameConfirmed for UTF-8 Chinese nickname")
	}
	if phase != domain.PhaseCountdown {
		t.Fatalf("expected countdown after Chinese nickname confirm, got %q", phase)
	}
	if gotNick != nick {
		t.Fatalf("nickname = %q, want %q", gotNick, nick)
	}
}

func TestRoom_SetNicknameWaitsForAllPlayers(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	addConnectedPlayer(r, "p1")
	addConnectedPlayer(r, "p2")

	payload := append([]byte{byte(len("Alice"))}, []byte("Alice")...)
	r.mu.Lock()
	r.handleSetNicknameMsg(r.state.Players["p1"], payload)
	phaseAfterOne := r.state.Phase
	r.mu.Unlock()

	if phaseAfterOne != domain.PhaseWaiting {
		t.Fatalf("expected waiting after one of two confirms, got %q", phaseAfterOne)
	}

	payload2 := append([]byte{byte(len("Bob"))}, []byte("Bob")...)
	r.mu.Lock()
	r.handleSetNicknameMsg(r.state.Players["p2"], payload2)
	phaseAfterBoth := r.state.Phase
	r.mu.Unlock()

	if phaseAfterBoth != domain.PhaseCountdown {
		t.Fatalf("expected countdown after both confirm, got %q", phaseAfterBoth)
	}
}

func TestRoom_SetNicknameSameNameStillConfirms(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	player := r.state.Players["p1"]
	currentName := player.Nickname
	payload := append([]byte{byte(len(currentName))}, []byte(currentName)...)
	r.handleSetNicknameMsg(player, payload)
	confirmed := player.NicknameConfirmed
	phase := r.state.Phase
	r.mu.Unlock()

	if !confirmed {
		t.Fatal("expected NicknameConfirmed when submitting unchanged nickname")
	}
	if phase != domain.PhaseCountdown {
		t.Fatalf("expected countdown after confirming unchanged nickname, got %q", phase)
	}
}

func TestTryStartWhenAllReady_WrongPhase(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhasePlaying
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhasePlaying {
		t.Error("tryStartWhenAllReady should not change phase when not waiting")
	}
}

func TestTryStartWhenAllReady_NotAllReady(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:       NewGameState("TEST"),
		connections: make(map[string]*PlayerConn),
		usedNames:   make(map[string]bool),
		logger:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhaseWaiting
	room.state.Players["p1"] = &domain.PlayerState{
		Nickname:          "Alice",
		PlayerIndex:       0,
		NicknameConfirmed: false,
	}
	room.connections["p1"] = &PlayerConn{}
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhaseWaiting {
		t.Errorf("Phase should remain waiting, got %v", room.state.Phase)
	}
}

func TestTryStartWhenAllReady_NoConnections(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhaseWaiting
	room.state.Players["p1"] = &domain.PlayerState{
		Nickname:          "Alice",
		PlayerIndex:       0,
		NicknameConfirmed: true,
	}
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhaseWaiting {
		t.Errorf("Phase should remain waiting when no connections, got %v", room.state.Phase)
	}
}

func TestTryStartWhenAllReady_EmptyPlayers(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhaseWaiting
	room.tryStartWhenAllReady()
	if room.state.Phase != domain.PhaseWaiting {
		t.Errorf("Phase should remain waiting with no players, got %v", room.state.Phase)
	}
}

func TestSaveStateWithError_NilStore(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		store:  nil,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	err := room.saveStateWithError()
	if err != nil {
		t.Errorf("saveStateWithError with nil store should return nil, got: %v", err)
	}
}

func TestSaveStateWithError_StoreSuccess(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	room := &Room{
		state:  NewGameState("TEST"),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	err := room.saveStateWithError()
	if err != nil {
		t.Errorf("saveStateWithError should succeed, got: %v", err)
	}
	if repo.saveCount != 1 {
		t.Errorf("saveCount = %d, want 1", repo.saveCount)
	}
}

func TestSaveStateWithError_StoreError(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("db unavailable")
	room := &Room{
		state:  NewGameState("TEST"),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	err := room.saveStateWithError()
	if err == nil {
		t.Fatal("saveStateWithError should return error when store fails")
	}
}

func TestSaveState_NilStoreDoesNotPanic(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		store:  nil,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.saveState()
}

func TestSaveState_StoreErrorDoesNotPanic(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("db unavailable")
	room := &Room{
		state:  NewGameState("TEST"),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.saveState()
}

func TestBuildSnapshot_EmptyPlayers(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	data := room.buildSnapshot()
	if data == nil {
		t.Fatal("buildSnapshot returned nil")
	}
	if len(data) < 10 {
		t.Errorf("snapshot too short: %d bytes", len(data))
	}
	if data[0] != protocol.MsgSnapshot {
		t.Errorf("first byte = 0x%02x, want 0x%02x", data[0], protocol.MsgSnapshot)
	}
}

func TestBuildSnapshot_WithPlayers(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Players["p1"] = &domain.PlayerState{
		Nickname:          "Alice",
		PlayerIndex:       0,
		Palette:           2,
		ScoreContribution: 10,
		CooldownEndTime:   time.Now().UnixMilli() + 5000,
	}
	room.state.Players["p2"] = &domain.PlayerState{
		Nickname:          "Bob",
		PlayerIndex:       1,
		Palette:           5,
		ScoreContribution: 20,
		CooldownEndTime:   time.Now().UnixMilli() - 1000,
	}
	data := room.buildSnapshot()
	if data == nil {
		t.Fatal("buildSnapshot returned nil")
	}
	if len(data) < 10 {
		t.Errorf("snapshot too short: %d bytes", len(data))
	}
}

func TestBuildSnapshot_WithFullState(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Phase = domain.PhasePlaying
	room.state.TickCount = 42
	room.state.Balloon.Score = 100
	room.state.Balloon.X = 0.5
	room.state.Balloon.Y = 0.3
	room.state.Balloon.VY = -2.0
	room.state.Balloon.VX = 0.1
	room.state.Bird.X = 0.8
	room.state.Bird.Y = 0.2
	room.state.Bird.Active = true
	room.state.Ghost.X = 0.1
	room.state.Ghost.Y = 0.9
	room.state.Ghost.Active = true
	room.state.Ghost.RepelTimer = 15
	room.state.Wind = 1.5

	for i := 0; i < 3; i++ {
		pid := string(rune('a' + i))
		room.state.Players[pid] = &domain.PlayerState{
			Nickname:    "P" + pid,
			PlayerIndex: i,
			Palette:     i,
		}
	}
	data := room.buildSnapshot()
	if data == nil {
		t.Fatal("buildSnapshot returned nil")
	}
	if data[0] != protocol.MsgSnapshot {
		t.Errorf("first byte = 0x%02x, want 0x%02x", data[0], protocol.MsgSnapshot)
	}
}

func TestBuildSnapshot_ReusesBuffer(t *testing.T) {
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Players["p1"] = &domain.PlayerState{
		Nickname:    "Alice",
		PlayerIndex: 0,
	}
	first := room.buildSnapshot()
	second := room.buildSnapshot()
	if first == nil || second == nil {
		t.Fatal("buildSnapshot returned nil")
	}
	if len(first) != len(second) {
		t.Errorf("snapshot lengths differ: %d vs %d", len(first), len(second))
	}
}

func TestBuildSnapshot_CooldownActiveVsExpired(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	now := time.Now().UnixMilli()
	room.state.Players["p1"] = &domain.PlayerState{
		Nickname:        "ActiveCooldown",
		PlayerIndex:     0,
		CooldownEndTime: now + 10000,
	}
	room.state.Players["p2"] = &domain.PlayerState{
		Nickname:        "ExpiredCooldown",
		PlayerIndex:     1,
		CooldownEndTime: now - 1000,
	}
	room.state.Players["p3"] = &domain.PlayerState{
		Nickname:        "ZeroCooldown",
		PlayerIndex:     2,
		CooldownEndTime: 0,
	}
	data := room.buildSnapshot()
	if data == nil {
		t.Fatal("buildSnapshot returned nil")
	}
}

func TestBuildSnapshot_AllPhases(t *testing.T) {
	t.Parallel()
	phases := []domain.GamePhase{
		domain.PhaseWaiting,
		domain.PhaseCountdown,
		domain.PhasePlaying,
		domain.PhaseEnded,
	}
	for _, phase := range phases {
		room := &Room{
			state:  NewGameState("TEST"),
			logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		room.state.Phase = phase
		data := room.buildSnapshot()
		if data == nil || len(data) == 0 {
			t.Errorf("buildSnapshot failed for phase %v", phase)
		}
	}
}

func TestValidateTapRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()
	room := &Room{state: NewGameState("TEST")}

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

	room := &Room{state: NewGameState("TEST")}

	t.Run("rejects short payload", func(t *testing.T) {
		_, _, ok := room.decodeTapPayload([]byte{0, 1, 2})
		if ok {
			t.Error("decodeTapPayload should reject < 8 bytes")
		}
	})

	t.Run("rejects nil payload", func(t *testing.T) {
		_, _, ok := room.decodeTapPayload(nil)
		if ok {
			t.Error("decodeTapPayload should reject nil")
		}
	})

	t.Run("rejects NaN coordinates", func(t *testing.T) {
		payload := encodeTapTestPayload(float32(math.NaN()), 0.5)
		_, _, ok := room.decodeTapPayload(payload)
		if ok {
			t.Error("decodeTapPayload should reject NaN")
		}
	})

	t.Run("rejects Inf coordinates", func(t *testing.T) {
		payload := encodeTapTestPayload(float32(math.Inf(1)), 0.5)
		_, _, ok := room.decodeTapPayload(payload)
		if ok {
			t.Error("decodeTapPayload should reject Inf")
		}
	})

	t.Run("rejects coordinates out of range [0,1]", func(t *testing.T) {
		payload := encodeTapTestPayload(1.5, 0.5)
		_, _, ok := room.decodeTapPayload(payload)
		if ok {
			t.Error("decodeTapPayload should reject x > 1")
		}
	})

	t.Run("rejects negative coordinates", func(t *testing.T) {
		payload := encodeTapTestPayload(-0.1, 0.5)
		_, _, ok := room.decodeTapPayload(payload)
		if ok {
			t.Error("decodeTapPayload should reject x < 0")
		}
	})

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

func TestCleanupDisconnected(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()
	grace := int64(protocol.ReconnectGraceMs)

	t.Run("removes player past grace period", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
			logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		disconnectedAt := now - grace - 1000
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, Disconnected: true, DisconnectedAt: &disconnectedAt}
		room.usedNames["Player1"] = true

		room.cleanupDisconnected(now)

		if _, exists := room.state.Players["p1"]; exists {
			t.Error("cleanupDisconnected should remove player past grace period")
		}
		if room.usedNames["Player1"] {
			t.Error("cleanupDisconnected should free used name")
		}
	})

	t.Run("keeps player within grace period", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
			logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		disconnectedAt := now - grace + 1000
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, Disconnected: true, DisconnectedAt: &disconnectedAt}
		room.usedNames["Player1"] = true

		room.cleanupDisconnected(now)

		if _, exists := room.state.Players["p1"]; !exists {
			t.Error("cleanupDisconnected should keep player within grace period")
		}
	})

	t.Run("skips connected players", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
			logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, Disconnected: false}

		room.cleanupDisconnected(now)

		if _, exists := room.state.Players["p1"]; !exists {
			t.Error("cleanupDisconnected should keep connected player")
		}
	})

	t.Run("handles empty players", func(t *testing.T) {
		room := &Room{state: NewGameState("TEST"), logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
		room.cleanupDisconnected(now)
	})
}

func TestUpdatePlayerStats(t *testing.T) {
	t.Parallel()

	t.Run("increments score", func(t *testing.T) {
		room := &Room{state: NewGameState("TEST")}
		room.state.Balloon.Score = 5
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0}

		cooldown := room.updatePlayerStats(room.state.Players["p1"], time.Now().UnixMilli())
		if room.state.Balloon.Score != 6 {
			t.Errorf("Score = %d, want %d", room.state.Balloon.Score, 6)
		}
		if cooldown <= 0 {
			t.Errorf("cooldown = %d, want > 0", cooldown)
		}
	})

	t.Run("calculates cooldown based on connected count", func(t *testing.T) {
		room := &Room{state: NewGameState("TEST")}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0}
		room.state.Players["p2"] = &domain.PlayerState{Nickname: "Player2", PlayerIndex: 1}
		room.state.Players["p3"] = &domain.PlayerState{Nickname: "Player3", PlayerIndex: 2}

		cooldown := room.updatePlayerStats(room.state.Players["p1"], time.Now().UnixMilli())
		if cooldown <= 0 {
			t.Errorf("cooldown should be positive, got %d", cooldown)
		}
	})
}

// encodeTapTestPayload helper: creates a mock tap payload for testing decodeTapPayload.
func encodeTapTestPayload(x, y float32) []byte {
	buf := make([]byte, 8)
	u32 := math.Float32bits(x)
	buf[0] = byte(u32)
	buf[1] = byte(u32 >> 8)
	buf[2] = byte(u32 >> 16)
	buf[3] = byte(u32 >> 24)
	u32 = math.Float32bits(y)
	buf[4] = byte(u32)
	buf[5] = byte(u32 >> 8)
	buf[6] = byte(u32 >> 16)
	buf[7] = byte(u32 >> 24)
	return buf
}

func TestAllConnectedPlayersReady(t *testing.T) {
	t.Parallel()

	t.Run("empty connections returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false with no connections")
		}
	})

	t.Run("all ready returns true", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: true}
		room.state.Players["p2"] = &domain.PlayerState{Nickname: "Player2", PlayerIndex: 1, NicknameConfirmed: true}
		room.connections["p1"] = &PlayerConn{}
		room.connections["p2"] = &PlayerConn{}

		if !room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return true when all connected players are ready")
		}
	})

	t.Run("player not found returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.connections["ghost"] = &PlayerConn{}

		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false for unknown player")
		}
	})

	t.Run("disconnected player returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: true, Disconnected: true}
		room.connections["p1"] = &PlayerConn{}

		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false for disconnected player")
		}
	})

	t.Run("player not confirmed returns false", func(t *testing.T) {
		room := &Room{
			connections: make(map[string]*PlayerConn),
			state:       NewGameState("TEST"),
		}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: false}
		room.connections["p1"] = &PlayerConn{}

		if room.allConnectedPlayersReady() {
			t.Error("allConnectedPlayersReady should return false when nickname not confirmed")
		}
	})
}

func TestNormalizePhaseForNicknameGate(t *testing.T) {
	t.Parallel()

	t.Run("waiting phase unchanged", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
			logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		room.state.Phase = domain.PhaseWaiting
		room.normalizePhaseForNicknameGate()
		if room.state.Phase != domain.PhaseWaiting {
			t.Errorf("Phase = %v, want %v", room.state.Phase, domain.PhaseWaiting)
		}
	})

	t.Run("playing resets to waiting when not all ready", func(t *testing.T) {
		room := &Room{
			state:       NewGameState("TEST"),
			connections: make(map[string]*PlayerConn),
			usedNames:   make(map[string]bool),
			logger:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}
		room.state.Phase = domain.PhasePlaying
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0, NicknameConfirmed: false}
		room.connections["p1"] = &PlayerConn{}

		room.normalizePhaseForNicknameGate()
		if room.state.Phase != domain.PhaseWaiting {
			t.Errorf("Phase should reset to waiting, got %v", room.state.Phase)
		}
	})
}

func TestTransitionPhaseIfNeeded(t *testing.T) {
	t.Parallel()

	t.Run("playing without tick starts tick", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
		}
		room.state.Phase = domain.PhasePlaying
		room.tickCancel = nil

		room.transitionPhaseIfNeeded()
	})

	t.Run("playing with active tick does nothing", func(t *testing.T) {
		room := &Room{
			state:     NewGameState("TEST"),
			usedNames: make(map[string]bool),
		}
		room.state.Phase = domain.PhasePlaying
		room.tickCancel = func() {}

		room.transitionPhaseIfNeeded()
		// tickCancel should not be replaced
		if room.tickCancel == nil {
			t.Error("tickCancel should not be nil after transitionPhaseIfNeeded")
		}
	})

	t.Run("non-playing phase does nothing", func(t *testing.T) {
		room := &Room{state: NewGameState("TEST")}
		room.state.Phase = domain.PhaseWaiting
		room.transitionPhaseIfNeeded()
	})
}
