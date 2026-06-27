package game

import (
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

	ch1 := make(chan []byte, 64)
	ch2 := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch1}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: ch2}
	r.mu.Unlock()

	msg := []byte{0x01, 0x02, 0x03}
	r.broadcast(msg, "")

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

	ch1 := make(chan []byte, 64)
	ch2 := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch1}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: ch2}
	r.mu.Unlock()

	msg := []byte{0x01, 0x02, 0x03}
	r.broadcast(msg, "p1")

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

// ─── HandleSetNickname ───────────────────────────────────────────────

func TestHandleSetNickname_FirstChange(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		PlayerIndex:        0,
		Nickname:           "OldName",
		LastNicknameChange: 0, // first change skips cooldown
	}
	usedNames := map[string]bool{"OldName": true}

	result := HandleSetNickname(state, player, "NewName", usedNames)

	if !result {
		t.Fatal("first nickname change should succeed")
	}
	if player.Nickname != "NewName" {
		t.Fatalf("nickname should be NewName, got %q", player.Nickname)
	}
	if usedNames["NewName"] != true {
		t.Fatal("NewName should be in usedNames")
	}
	if usedNames["OldName"] != false {
		t.Fatal("OldName should be removed from usedNames")
	}
}

func TestHandleSetNickname_SameName(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "SameName",
		LastNicknameChange: 0,
	}
	usedNames := map[string]bool{"SameName": true}

	result := HandleSetNickname(state, player, "SameName", usedNames)

	if result {
		t.Fatal("same nickname should return false")
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

func TestHandleSetNickname_DuplicateName(t *testing.T) {
	state := NewGameState("TEST")
	player := &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "OldName",
		LastNicknameChange: 0,
	}
	usedNames := map[string]bool{"OldName": true, "TakenName": true}

	result := HandleSetNickname(state, player, "TakenName", usedNames)

	if !result {
		t.Fatal("duplicate name should still succeed (generates unique name)")
	}
	if player.Nickname == "TakenName" {
		t.Fatal("should not use the duplicate name directly")
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

// ─── HandleRestartVote ───────────────────────────────────────────────

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

func TestRoom_NormalizePhaseForNicknameGate(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	r.state.Phase = domain.PhaseCountdown
	r.normalizePhaseForNicknameGate()
	phase := r.state.Phase
	r.mu.Unlock()

	if phase != domain.PhaseWaiting {
		t.Fatalf("expected waiting when countdown without nickname confirm, got %q", phase)
	}
}

// ─── RestartAndStart ─────────────────────────────────────────────────

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
