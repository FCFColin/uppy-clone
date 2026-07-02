package game

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/store"
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

func TestRoom_notifyJoin_Reconnect(t *testing.T) {
	r := NewRoom("JOIN2", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "Bob", PlayerIndex: 0, Palette: 2}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.notifyJoin("p1", player, true)
	select {
	case <-r.connections["p1"].Send:
	default:
		t.Fatal("reconnect notifyJoin should send snapshot")
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

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		time.Sleep(2 * time.Second)
		conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Conn: conn, Send: make(chan []byte, 4)}

	r.closeExistingConnection("p1", player)
	if _, ok := r.connections["p1"]; ok {
		t.Fatal("closeExistingConnection should remove old connection")
	}
}

func TestRoom_closeExistingConnection_SkipsNilPlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}

	r.closeExistingConnection("p1", nil)
	if _, ok := r.connections["p1"]; !ok {
		t.Fatal("nil player should not trigger connection replacement")
	}
}

func TestRoom_closeExistingConnection_SkipsDisconnected(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	player := &domain.PlayerState{ID: "p1", Disconnected: true}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}

	r.closeExistingConnection("p1", player)
	if _, ok := r.connections["p1"]; !ok {
		t.Fatal("disconnected player should not trigger connection replacement")
	}
}

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

	disconnectedAt := time.Now().UnixMilli() - domain.ReconnectGraceMs - 1000 // expired
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

func BenchmarkRoom_CleanupDisconnected(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("BENCH", nil, nil, timeouts, 0)

	now := time.Now().UnixMilli()
	r.mu.Lock()
	for i := 0; i < 100; i++ {
		pid := "p" + string(rune('0'+i%10)) + string(rune('0'+i/10))
		disconnectedAt := now - domain.ReconnectGraceMs - 1000
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

// ─── Nickname ready flow ─────────────────────────────────────────────

// waitForCountdown polls the room phase until it becomes countdown or timeout.
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
	if len(nick) <= config.MaxNicknameLen {
		t.Fatalf("test nickname byte length = %d, want > %d to reproduce the bug", len(nick), config.MaxNicknameLen)
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

func TestCleanupDisconnected(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()
	grace := int64(domain.ReconnectGraceMs)

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

func TestRoom_HandleJoin_ExistingPlayer(t *testing.T) {
	r := NewRoom("JOIN3", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Players["p1"] = &domain.PlayerState{
		ID: "p1", Nickname: "Alice", PlayerIndex: 0, NicknameConfirmed: true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up := websocket.Upgrader{}
		up.Upgrade(w, req, nil)
	}))
	defer server.Close()
	wsURL := "ws" + server.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up := websocket.Upgrader{}
		up.Upgrade(w, req, nil)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := r.HandleJoin("p1", conn); err != nil {
		t.Fatalf("HandleJoin reconnect: %v", err)
	}
	if r.state.Players["p1"].Disconnected {
		t.Fatal("player should be reconnected")
	}
}

func TestRoom_reconnectPlayer_CountdownPhase(t *testing.T) {
	r := NewRoom("RCD", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseCountdown
	r.countdownStart = time.Now().UnixMilli()
	player := &domain.PlayerState{ID: "p1", Nickname: "Nick", PlayerIndex: 0}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.reconnectPlayer("p1", player)
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

func TestRoom_normalizePhaseForNicknameGate_Countdown(t *testing.T) {
	r := NewRoom("NG1", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseCountdown
	r.endGameTimer = time.AfterFunc(time.Hour, func() {})
	r.startDelayTimer = time.AfterFunc(time.Hour, func() {})
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", NicknameConfirmed: false}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1"}
	r.normalizePhaseForNicknameGate()
	if r.state.Phase != domain.PhaseWaiting {
		t.Fatalf("phase = %s", r.state.Phase)
	}
}

func TestRoom_tryStartWhenAllReady_AlreadyScheduled(t *testing.T) {
	r := NewRoom("TSR", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseWaiting
	r.startDelayTimer = time.AfterFunc(time.Hour, func() {})
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", NicknameConfirmed: true}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1"}
	r.tryStartWhenAllReady()
}

func TestRoom_setEndGameAlarm_EndedPhase(t *testing.T) {
	r := NewRoom("EGA", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseEnded
	addConnectedPlayer(r, "p1")
	r.setEndGameAlarm(time.Now().Add(10 * time.Millisecond))
	time.Sleep(50 * time.Millisecond)
}

func TestRoom_handleCountdownEnd_WithStore(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectExec("INSERT INTO game_sessions").WillReturnResult(pgconn.NewCommandTag("INSERT 1"))

	r := NewRoom("HCE", nil, db, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseCountdown
	r.state.SessionID = "11111111-1111-4111-8111-111111111111"
	r.state.StartedAt = time.Now().UnixMilli()
	r.handleCountdownEnd()
}

func TestRoom_HandleJoin_NewPlayer(t *testing.T) {
	r := NewRoom("NEW1", nil, nil, config.DefaultTimeoutConfig(), 4)
	if err := r.HandleJoin("p1", nil); err != nil {
		t.Fatalf("HandleJoin: %v", err)
	}
	if _, ok := r.state.Players["p1"]; !ok {
		t.Fatal("new player should be added")
	}
}

func TestRoom_reconnectPlayer_WaitingPhase(t *testing.T) {
	r := NewRoom("RW", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.startDelay = time.Millisecond
	r.state.Phase = domain.PhaseWaiting
	player := &domain.PlayerState{ID: "p1", Nickname: "Nick", NicknameConfirmed: true}
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.reconnectPlayer("p1", player)
	time.Sleep(20 * time.Millisecond)
}

func TestRoom_normalizePhaseForNicknameGate_AllReadyNoOp(t *testing.T) {
	r := NewRoom("NR", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhasePlaying
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", NicknameConfirmed: true}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1"}
	r.normalizePhaseForNicknameGate()
	if r.state.Phase != domain.PhasePlaying {
		t.Fatalf("phase = %s, want playing when all ready", r.state.Phase)
	}
}

func TestRoom_setEndGameAlarm_ReplacesExisting(t *testing.T) {
	r := NewRoom("RT", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.endGameTimer = time.AfterFunc(time.Hour, func() {})
	r.state.Phase = domain.PhaseEnded
	r.setEndGameAlarm(time.Now().Add(time.Millisecond))
	time.Sleep(20 * time.Millisecond)
}

func TestRoom_setEndGameAlarm_PastDeadline(t *testing.T) {
	r := NewRoom("PD", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseCountdown
	r.setEndGameAlarm(time.Now().Add(-time.Second))
	time.Sleep(20 * time.Millisecond)
	if r.state.Phase != domain.PhasePlaying {
		t.Fatalf("phase = %s, want playing after past deadline", r.state.Phase)
	}
}

func TestRoom_handleAutoRestart_AutoRestart(t *testing.T) {
	r := NewRoom("AR3", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseEnded
	addConnectedPlayer(r, "p1")
	r.state.RestartVotes = map[string]bool{}
	r.handleAutoRestart()
	if r.state.Phase != domain.PhaseCountdown {
		t.Fatalf("phase = %s, want countdown after auto restart", r.state.Phase)
	}
}

func TestRoom_Close_WithTickAndConnections(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("CL1", nil, repo, config.DefaultTimeoutConfig(), 4)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4), Conn: nil}
	r.endGameTimer = time.AfterFunc(time.Hour, func() {})
	r.startDelayTimer = time.AfterFunc(time.Hour, func() {})
	r.mu.Unlock()
	r.startTick()
	r.Close()
}
