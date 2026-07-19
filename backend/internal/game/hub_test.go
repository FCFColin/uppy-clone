package game

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── NewHub ──────────────────────────────────────────────────────────

func TestNewHub(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)
	if h == nil {
		t.Fatal("NewHub returned nil")
	}
	if h.RoomCount() != 0 {
		t.Fatalf("expected 0 rooms, got %d", h.RoomCount())
	}
}

// ─── CreateRoom ──────────────────────────────────────────────────────

func TestHub_CreateRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if len(code) != 5 {
		t.Fatalf("expected 5-char room code, got %q (len=%d)", code, len(code))
	}
	if h.RoomCount() != 1 {
		t.Fatalf("expected 1 room after CreateRoom, got %d", h.RoomCount())
	}
}

// ─── GetRoom ─────────────────────────────────────────────────────────

func TestHub_GetRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	t.Run("Found", func(t *testing.T) {
		code, _ := h.CreateRoom(context.Background())
		room := h.getRoom(code)
		if room == nil {
			t.Fatal("expected to find room by code")
		}
		if string(room.state.LobbyCode) != code {
			t.Fatalf("room code mismatch: got %q, want %q", string(room.state.LobbyCode), code)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		room := h.getRoom("NOPE1")
		if room != nil {
			t.Fatal("expected nil for nonexistent room (no store)")
		}
	})
}

// ─── RemoveRoom ──────────────────────────────────────────────────────

func TestHub_RemoveRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	if h.RoomCount() != 1 {
		t.Fatalf("expected 1 room, got %d", h.RoomCount())
	}

	h.RemoveRoom(context.Background(), code)
	if h.RoomCount() != 0 {
		t.Fatalf("expected 0 rooms after RemoveRoom, got %d", h.RoomCount())
	}

	h.RemoveRoom(context.Background(), "NOPE1")
}

// ─── CheckRoom ───────────────────────────────────────────────────────

func TestHub_CheckRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	t.Run("Existing", func(t *testing.T) {
		code, _ := h.CreateRoom(context.Background())
		info, err := h.CheckRoom(code)
		if err != nil {
			t.Fatalf("CheckRoom failed: %v", err)
		}
		if info == nil {
			t.Fatal("expected RoomInfo for existing room")
		}
		if info.Code != code {
			t.Fatalf("room code mismatch: got %q, want %q", info.Code, code)
		}
		if info.Phase != string(domain.PhaseWaiting) {
			t.Fatalf("expected phase waiting, got %q", info.Phase)
		}
	})

	t.Run("Nonexistent", func(t *testing.T) {
		info, err := h.CheckRoom("NOPE1")
		if err != nil {
			t.Fatalf("CheckRoom failed: %v", err)
		}
		if info != nil {
			t.Fatal("expected nil for nonexistent room")
		}
	})
}

// ─── CleanupLoop ─────────────────────────────────────────────────────

func TestHub_CleanupLoop(t *testing.T) {
	cases := []struct {
		name          string
		setup         func(room *Room)
		wantRoomCount int
	}{
		{
			name:          "RemovesEmptyRooms",
			setup:         func(_ *Room) {},
			wantRoomCount: 0,
		},
		{
			name: "KeepsRoomWithConnections",
			setup: func(room *Room) {
				room.mu.Lock()
				room.connections["player1"] = &PlayerConn{PlayerID: "player1", Send: make(chan []byte, 64)}
				room.mu.Unlock()
			},
			wantRoomCount: 1,
		},
		{
			name: "RemovesAllDisconnectedExpired",
			setup: func(room *Room) {
				disconnectedAt := time.Now().UnixMilli() - domain.ReconnectGraceMs - 1000
				room.mu.Lock()
				room.state.Players["p1"] = &domain.PlayerState{
					ID:             "p1",
					Nickname:       "gone",
					Disconnected:   true,
					DisconnectedAt: &disconnectedAt,
				}
				room.mu.Unlock()
			},
			wantRoomCount: 0,
		},
		{
			name: "RemovesZeroPlayerRoom",
			setup: func(room *Room) {
				room.mu.Lock()
				room.state.Phase = domain.PhasePlaying
				room.mu.Unlock()
			},
			wantRoomCount: 0,
		},
		{
			name: "KeepsDisconnectedInGrace",
			setup: func(room *Room) {
				disconnectedAt := time.Now().UnixMilli() - 1000
				room.mu.Lock()
				room.state.Phase = domain.PhasePlaying
				room.state.Players["p1"] = &domain.PlayerState{
					ID:             "p1",
					Nickname:       "grace",
					Disconnected:   true,
					DisconnectedAt: &disconnectedAt,
				}
				room.mu.Unlock()
			},
			wantRoomCount: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			timeouts := config.DefaultTimeoutConfig()
			h := NewHub(nil, nil, timeouts, 0, 0)
			code, _ := h.CreateRoom(context.Background())
			room := h.getRoom(code)
			c.setup(room)
			h.cleanupOnce()
			if h.RoomCount() != c.wantRoomCount {
				t.Fatalf("RoomCount = %d, want %d", h.RoomCount(), c.wantRoomCount)
			}
		})
	}
}

func TestHub_CleanupLoop_ContextCancellation(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		h.CleanupLoop(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CleanupLoop did not exit after context cancellation")
	}
}

// ─── Timeouts ────────────────────────────────────────────────────────

func TestHub_Timeouts(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	got := h.Timeouts()
	if got.PGConnectTimeout != timeouts.PGConnectTimeout {
		t.Fatalf("Timeouts mismatch: got %v, want %v", got.PGConnectTimeout, timeouts.PGConnectTimeout)
	}
}

// ─── Broadcast backpressure ─────────────────────────────────────────

func TestRoom_Broadcast_Backpressure(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 50)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.syncOutbound = true

	pc := &PlayerConn{
		PlayerID: "player1",
		Send:     make(chan []byte, config.WSChannelBuffer),
	}
	room.mu.Lock()
	room.connections["player1"] = pc
	room.mu.Unlock()

	for i := 0; i < config.WSChannelBuffer; i++ {
		pc.Send <- []byte{protocol.MsgSnapshot}
	}

	done := make(chan struct{})
	go func() {
		room.mu.Lock()
		room.broadcast([]byte{protocol.MsgSnapshot}, "")
		room.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("broadcast blocked when Send channel was full")
	}

	if len(pc.Send) != config.WSChannelBuffer {
		t.Fatalf("expected Send channel to remain full (len=%d), got len=%d",
			config.WSChannelBuffer, len(pc.Send))
	}
}

// ─── Bulkhead: WS Connection Limit ──────────────────────────────────

func TestHub_WSConnectionLimit(t *testing.T) {
	t.Run("RejectsWhenFull", func(t *testing.T) {
		timeouts := config.DefaultTimeoutConfig()
		h := NewHub(nil, nil, timeouts, 5, 50) // max 5 WS connections

		for i := 0; i < 5; i++ {
			if !h.CanAcceptWSConnection() {
				t.Fatalf("should accept connection %d", i)
			}
			h.IncrementWSConnection()
		}

		if h.CanAcceptWSConnection() {
			t.Fatal("should reject connection when limit reached")
		}

		if count := h.WSConnCount(); count != 5 {
			t.Fatalf("expected 5 connections, got %d", count)
		}
	})

	t.Run("DefaultValues", func(t *testing.T) {
		timeouts := config.DefaultTimeoutConfig()
		h := NewHub(nil, nil, timeouts, 0, 0) // zero → should use defaults

		if h.MaxWSConnections() != 1000 {
			t.Fatalf("expected default max 1000, got %d", h.MaxWSConnections())
		}
		if h.MaxPlayersPerRoom() != 50 {
			t.Fatalf("expected default max players 50, got %d", h.MaxPlayersPerRoom())
		}
	})
}

// ─── Bulkhead: Room MaxPlayers ──────────────────────────────────────

func TestRoom_MaxPlayers_RejectsWhenFull(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 3)

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)

	room.mu.Lock()
	for i := 0; i < 3; i++ {
		pid := fmt.Sprintf("player%d", i)
		room.state.Players[pid] = &domain.PlayerState{
			ID:          pid,
			PlayerIndex: i,
			Nickname:    fmt.Sprintf("nick%d", i),
		}
		room.usedNames[fmt.Sprintf("nick%d", i)] = true
	}
	room.mu.Unlock()

	err := room.HandleJoin("player3", nil)
	if err != ErrRoomFull {
		t.Fatalf("expected ErrRoomFull, got %v", err)
	}
}

func TestRoom_MaxPlayers_ReconnectDoesNotCount(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 2) // max 2 players per room

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)

	room.mu.Lock()
	for i := 0; i < 2; i++ {
		pid := fmt.Sprintf("player%d", i)
		room.state.Players[pid] = &domain.PlayerState{
			ID:          pid,
			PlayerIndex: i,
			Nickname:    fmt.Sprintf("nick%d", i),
		}
		room.usedNames[fmt.Sprintf("nick%d", i)] = true
	}

	room.state.Players["player0"].Disconnected = true
	now := time.Now().UnixMilli()
	room.state.Players["player0"].DisconnectedAt = &now
	room.mu.Unlock()

	err := room.HandleJoin("player0", nil)
	if err != nil {
		t.Fatalf("reconnect should succeed, got %v", err)
	}
}

// ─── RestoreRooms (nil store) ────────────────────────────────────────

func TestHub_RestoreRooms_NilStore(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("expected nil error with nil store, got %v", err)
	}
}

func TestMatchRoom_NoRooms(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 2, 4)

	code, err := h.MatchRoom(context.Background())
	if err != nil {
		t.Fatalf("MatchRoom on empty hub: %v", err)
	}
	if code == "" {
		t.Fatal("MatchRoom returned empty code")
	}
}

func TestMatchRoom_FullRoomsCreateNew(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 2, 1)

	code1, _ := h.CreateRoom(context.Background())
	room1 := h.getRoom(code1)

	room1.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0}
	room1.state.Phase = domain.PhasePlaying

	code2, err := h.MatchRoom(context.Background())
	if err != nil {
		t.Fatalf("MatchRoom: %v", err)
	}
	if code2 == code1 {
		t.Error("MatchRoom should create a new room when all rooms are full")
	}
}

func TestInstanceAddress(t *testing.T) {
	if err := os.Setenv("INSTANCE_ADDR", "10.0.0.1:9000"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("INSTANCE_ADDR") }()
	addr := instanceAddress()
	if addr != "10.0.0.1:9000" {
		t.Errorf("instanceAddress = %q, want %q", addr, "10.0.0.1:9000")
	}
}

func TestHub_CreateRoom_CodeConflict(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)
	h.rooms["CONFL"] = NewRoom("CONFL", h, nil, timeouts, 0)

	restore := h.SetGenerateRoomCodeHook(func() string { return "CONFL" })
	defer restore()

	_, err := h.CreateRoom(context.Background())
	if err != ErrRoomCodeConflict {
		t.Fatalf("CreateRoom error = %v, want ErrRoomCodeConflict", err)
	}
}

func TestHub_CreateRoom_WithRedis(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)

	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if h.getRoom(code) == nil {
		t.Fatal("room should exist after CreateRoom")
	}

	ctx := context.Background()
	info, err := redisStore.GetRoomRegistry(ctx, code)
	if err != nil {
		t.Fatalf("GetRoomRegistry: %v", err)
	}
	if info == nil || info.Code != code {
		t.Fatalf("registry info = %+v, want code %q", info, code)
	}
}

func TestHub_RemoveRoom_DeletesFromStore(t *testing.T) {
	repo := newMockRoomRepository()
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(repo, nil, timeouts, 0, 0)

	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	h.RemoveRoom(context.Background(), code)

	if repo.deleteCount != 1 {
		t.Fatalf("deleteCount = %d, want 1", repo.deleteCount)
	}
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0", h.RoomCount())
	}
}

func TestHub_removeRooms_EmptyBatch(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	h.removeRooms(nil, removeRoomOptions{pgDelete: true})
	h.removeRooms([]string{"MISSING"}, removeRoomOptions{pgDelete: true})
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0", h.RoomCount())
	}
}

func TestHub_CleanupLoop_RunsCleanupOnce(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhaseWaiting
	room.connections = make(map[string]*PlayerConn)
	room.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.CleanupLoop(ctx)
		close(done)
	}()
	h.cleanupOnce()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CleanupLoop did not exit")
	}
	if h.getRoom(code) != nil {
		t.Fatal("empty waiting room should be cleaned up")
	}
}
