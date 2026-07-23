package game

import (
	"sync"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

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

func TestRoom_StartGame_FromWaiting(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

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

func TestReconnectPlayer_Countdown(t *testing.T) {
	r := NewRoom("RP", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseCountdown
	r.countdownStart = time.Now().UnixMilli()
	player := &domain.PlayerState{ID: "p1", Nickname: "p1", Disconnected: true}
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.reconnectPlayer("p1", player)
	if player.Disconnected {
		t.Fatal("player should be reconnected")
	}
}

func TestRoom_handleCountdownEnd(t *testing.T) {
	r := NewRoom("CD1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseCountdown
	r.state.SessionID = "sess-1"
	r.state.StartedAt = time.Now().UnixMilli()
	r.handleCountdownEnd()
	if r.state.Phase != domain.PhasePlaying {
		t.Fatalf("phase = %s", r.state.Phase)
	}
	if r.state.TickCount != 1 {
		t.Fatalf("TickCount = %d", r.state.TickCount)
	}
}

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

func TestRoom_ConcurrentMessagesUnderPlaying(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("CONT", nil, nil, timeouts, 50)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2", Nickname: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 256)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 256)}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			playerID := "p1"
			if id%2 == 1 {
				playerID = "p2"
			}
			start := time.Now()
			if err := r.HandleMessage(playerID, protocol.MsgPing, nil); err != nil {
				t.Errorf("HandleMessage error: %v", err)
			}
			if d := time.Since(start); d > 25*time.Millisecond {
				t.Errorf("HandleMessage blocked %v", d)
			}
		}(i)
	}
	wg.Wait()

	// Avoid Close() here: test PlayerConns have nil websocket handles.
	r.mu.Lock()
	r.stopTick()
	r.mu.Unlock()
}
