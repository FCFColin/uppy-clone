package game

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestRoom_Code(t *testing.T) {
	t.Parallel()
	r := NewRoom("ABCD1", nil, nil, config.DefaultTimeoutConfig(), 0)
	if got := r.Code(); got != "ABCD1" {
		t.Fatalf("Code() = %q, want ABCD1", got)
	}
}

func TestRoom_notifyJoin_SendsSnapshot(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("JOIN1", nil, nil, timeouts, 0)
	player := &domain.PlayerState{
		ID: "p1", Nickname: "Alice", PlayerIndex: 0, Palette: 1, NicknameConfirmed: true,
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.notifyJoin("p1", player, false)
	select {
	case msg := <-r.connections["p1"].Send:
		if len(msg) == 0 {
			t.Fatal("expected snapshot message")
		}
	default:
		t.Fatal("notifyJoin should enqueue snapshot to joining player")
	}
}

func TestRoom_addNewPlayer_Success(t *testing.T) {
	r := NewRoom("ADD1", nil, nil, config.DefaultTimeoutConfig(), 4)
	player, err := r.addNewPlayer("p1", nil)
	if err != nil {
		t.Fatalf("addNewPlayer: %v", err)
	}
	if player == nil || player.ID != "p1" {
		t.Fatalf("player = %+v", player)
	}
	if len(r.state.Players) != 1 {
		t.Fatalf("players = %d, want 1", len(r.state.Players))
	}
}

func TestRoom_addNewPlayer_RoomFull(t *testing.T) {
	r := NewRoom("FULL", nil, nil, config.DefaultTimeoutConfig(), 1)
	r.state.Players["p0"] = &domain.PlayerState{ID: "p0", Nickname: "Taken"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1)}

	_, err := r.addNewPlayer("p1", nil)
	if err != ErrRoomFull {
		t.Fatalf("err = %v, want ErrRoomFull", err)
	}
	if _, ok := r.connections["p1"]; ok {
		t.Fatal("full room should remove pending connection")
	}
}

func TestRoom_closeExistingConnection_ClosesAndRemoves(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	player := &domain.PlayerState{ID: "p1", Disconnected: false}

	serverCloseCh := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		<-serverCloseCh
		_ = conn.Close()
	}))
	defer server.Close()
	defer close(serverCloseCh)

	wsURL := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Conn: conn, Send: make(chan []byte, 4)}

	r.closeExistingConnection("p1", player)
	if _, ok := r.connections["p1"]; ok {
		t.Fatal("closeExistingConnection should remove old connection")
	}
}

func TestRoom_Creation(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	if r == nil {
		t.Fatal("NewRoom returned nil")
	}
	if string(r.state.LobbyCode) != "TEST1" {
		t.Fatalf("expected LobbyCode TEST1, got %q", string(r.state.LobbyCode))
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

// ─── broadcast ───────────────────────────────────────────────────────

func TestRoom_Close(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:       "p1",
		Nickname: "TestPlayer",
	}
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
		got := protocol.GamePhase(tt.input)
		if got != tt.expected {
			t.Errorf("protocol.GamePhase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ─── Nickname ready flow ─────────────────────────────────────────────

func waitForCountdown(t *testing.T, r *Room, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.RLock()
		phase := r.state.Phase
		r.mu.RUnlock()
		if phase == domain.PhaseCountdown {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.mu.RLock()
	phase := r.state.Phase
	r.mu.RUnlock()
	t.Fatalf("timed out waiting for countdown phase, got %q", phase)
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
	r.startDelay = 10 * time.Millisecond
	addConnectedPlayer(r, "p1")

	payload := append([]byte{byte(len("Alice"))}, []byte("Alice")...)
	r.mu.Lock()
	player := r.state.Players["p1"]
	r.handleSetNicknameMsg(player, payload)
	r.mu.Unlock()

	waitForCountdown(t, r, 2*time.Second)
}

func TestRoom_SetNicknameChineseUTF8StartsCountdown(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.startDelay = 10 * time.Millisecond
	addConnectedPlayer(r, "p1")

	nick := "好奇的中子"
	payload := append([]byte{byte(len(nick))}, []byte(nick)...)
	if len(nick) <= domain.MaxNicknameLen {
		t.Fatalf("test nickname byte length = %d, want > %d to reproduce the bug", len(nick), domain.MaxNicknameLen)
	}

	r.mu.Lock()
	player := r.state.Players["p1"]
	r.handleSetNicknameMsg(player, payload)
	confirmed := player.NicknameConfirmed
	gotNick := player.Nickname
	r.mu.Unlock()

	if !confirmed {
		t.Fatal("expected NicknameConfirmed for UTF-8 Chinese nickname")
	}
	if gotNick != nick {
		t.Fatalf("nickname = %q, want %q", gotNick, nick)
	}
	waitForCountdown(t, r, 2*time.Second)
}

func TestRoom_SetNicknameWaitsForAllPlayers(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.startDelay = 10 * time.Millisecond
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
	r.mu.Unlock()

	waitForCountdown(t, r, 2*time.Second)
}

func TestRoom_SetNicknameSameNameStillConfirms(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.startDelay = 10 * time.Millisecond
	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	player := r.state.Players["p1"]
	currentName := player.Nickname
	payload := append([]byte{byte(len(currentName))}, []byte(currentName)...)
	r.handleSetNicknameMsg(player, payload)
	confirmed := player.NicknameConfirmed
	r.mu.Unlock()

	if !confirmed {
		t.Fatal("expected NicknameConfirmed when submitting unchanged nickname")
	}
	waitForCountdown(t, r, 2*time.Second)
}

func TestRoom_HandleJoin_ExistingPlayer(t *testing.T) {
	r := NewRoom("JOIN3", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Players["p1"] = &domain.PlayerState{
		ID: "p1", Nickname: "Alice", PlayerIndex: 0, NicknameConfirmed: true,
	}

	server := testutil.NewWSTestUpgraderServer(t)
	wsURL := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := r.HandleJoin("p1", conn); err != nil {
		t.Fatalf("HandleJoin: %v", err)
	}
}

func TestRoom_HandleJoin_ReconnectDuringGrace(t *testing.T) {
	r := NewRoom("RECON", nil, nil, config.DefaultTimeoutConfig(), 4)
	now := time.Now().UnixMilli()
	r.state.Players["p1"] = &domain.PlayerState{
		ID: "p1", Nickname: "Bob", PlayerIndex: 0, Disconnected: true, DisconnectedAt: &now,
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}

	server := testutil.NewWSTestUpgraderServer(t)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := r.HandleJoin("p1", conn); err != nil {
		t.Fatalf("HandleJoin reconnect: %v", err)
	}
	if r.state.Players["p1"].Disconnected {
		t.Fatal("player should be reconnected")
	}
}

func TestRoom_reconnectPlayer_PlayingPhase(t *testing.T) {
	r := NewRoom("RPL", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhasePlaying
	player := &domain.PlayerState{ID: "p1", Nickname: "Nick", PlayerIndex: 0}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.reconnectPlayer("p1", player)
	if r.tickCancel == nil {
		t.Fatal("expected tick started on playing reconnect")
	}
	r.stopTick()
}
