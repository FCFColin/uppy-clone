package game

import (
	"math"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestRoom_HandleMessage_RateLimit(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)

	// Set MessageCount below rate limit to verify the rate-limiting logic
	// without triggering the Close() call on nil websocket.Conn
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "TestPlayer",
		MessageCount:       domain.MessageRateLimit - 1,
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
	if count != domain.MessageRateLimit {
		t.Fatalf("expected MessageCount=%d, got %d", domain.MessageRateLimit, count)
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

func TestRoom_HandleMessage_TapAndRestartVote(t *testing.T) {
	r := NewRoom("TAP1", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhasePlaying
	now := time.Now().UnixMilli()
	r.state.Players["p1"] = &domain.PlayerState{
		ID: "p1", Nickname: "Tap", CooldownEndTime: now - 1,
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}

	var tapPayload [9]byte
	tapPayload[0] = protocol.MsgTap
	if err := r.HandleMessage("p1", protocol.MsgTap, tapPayload[:]); err != nil {
		t.Fatalf("tap HandleMessage: %v", err)
	}
	if err := r.HandleMessage("p1", protocol.MsgRestartVote, []byte{protocol.MsgRestartVote}); err != nil {
		t.Fatalf("restart vote HandleMessage: %v", err)
	}
}

func TestRoom_HandleMessage_SetNickname(t *testing.T) {
	r := NewRoom("NICK1", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "Old"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}

	nick := "NewNick"
	payload := append([]byte{byte(len(nick))}, []byte(nick)...)
	msg := append([]byte{protocol.MsgSetNickname}, payload...)
	if err := r.HandleMessage("p1", protocol.MsgSetNickname, msg); err != nil {
		t.Fatalf("set nickname HandleMessage: %v", err)
	}
}

// ─── modelPhaseToProtocol ────────────────────────────────────────────

func TestValidateTapRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()
	room := &Room{state: NewGameState("TEST", 42, testRNG())}

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

	room := &Room{state: NewGameState("TEST", 42, testRNG())}

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

func TestUpdatePlayerStats(t *testing.T) {
	t.Parallel()

	t.Run("increments score", func(t *testing.T) {
		room := &Room{state: NewGameState("TEST", 42, testRNG())}
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
		room := &Room{state: NewGameState("TEST", 42, testRNG())}
		room.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0}
		room.state.Players["p2"] = &domain.PlayerState{Nickname: "Player2", PlayerIndex: 1}
		room.state.Players["p3"] = &domain.PlayerState{Nickname: "Player3", PlayerIndex: 2}

		cooldown := room.updatePlayerStats(room.state.Players["p1"], time.Now().UnixMilli())
		if cooldown <= 0 {
			t.Errorf("cooldown should be positive, got %d", cooldown)
		}
	})
}

func TestRoom_tickOnce_NotPlaying(t *testing.T) {
	r := NewRoom("TICK1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.state.Phase = domain.PhaseWaiting
	r.state.TickCount = 5
	r.tickOnce(time.Now())
	if r.state.TickCount != 5 {
		t.Fatalf("TickCount = %d, want 5 when not playing", r.state.TickCount)
	}
	r.mu.Unlock()
}

func TestRoom_tickOnce_AdvancesPlaying(t *testing.T) {
	r := NewRoom("TICK2", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.TickCount = 0
	r.tickOnce(time.Now())
	if r.state.TickCount != 1 {
		t.Fatalf("TickCount = %d, want 1", r.state.TickCount)
	}
	r.mu.Unlock()
}

func TestRoom_tickOnce_GameOverGround(t *testing.T) {
	r := NewRoom("TICK3", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.Balloon.Y = 0.001
	r.state.Balloon.VY = -0.1
	r.tickOnce(time.Now())
	phase := r.state.Phase
	r.mu.Unlock()
	if phase != domain.PhaseEnded {
		t.Fatalf("phase = %s, want ended after ground collision", phase)
	}
}

func TestRoom_tickOnce_GhostCollision(t *testing.T) {
	r := NewRoom("TICK4", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.Ghost.Active = true
	r.state.Ghost.X = r.state.Balloon.X
	r.state.Ghost.Y = r.state.Balloon.Y
	r.state.Ghost.VX = 0
	r.state.Ghost.VY = 0
	r.state.Ghost.RepelTimer = 0
	r.tickOnce(time.Now())
	phase := r.state.Phase
	r.mu.Unlock()
	if phase != domain.PhaseEnded {
		t.Fatalf("phase = %s, want ended after ghost collision", phase)
	}
}

func TestRoom_tickOnce_BirdCollision(t *testing.T) {
	r := NewRoom("TICK5", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.Bird.Active = true
	r.state.Bird.X = r.state.Balloon.X
	r.state.Bird.Y = r.state.Balloon.Y
	r.state.Bird.VX = 0
	r.state.Bird.VY = 0
	r.tickOnce(time.Now())
	phase := r.state.Phase
	r.mu.Unlock()
	if phase != domain.PhaseEnded {
		t.Fatalf("phase = %s, want ended after bird collision", phase)
	}
}

func TestRoom_tickOnce_StopsWhenAllDisconnected(t *testing.T) {
	r := NewRoom("TICK6", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	now := time.Now().UnixMilli()
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Disconnected: true, DisconnectedAt: &now}
	r.startTick()
	if r.tickCancel == nil {
		t.Fatal("expected tick to start")
	}
	r.tickOnce(time.Now())
	if r.tickCancel != nil {
		t.Fatal("expected tick to stop when all players disconnected")
	}
	r.mu.Unlock()
}

func TestRoom_tickOnce_EmptyPlayersStopsTick(t *testing.T) {
	r := NewRoom("TICK7", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.startTick()
	r.tickOnce(time.Now())
	if r.tickCancel != nil {
		t.Fatal("expected tick to stop with zero players")
	}
	r.mu.Unlock()
}

func TestRoom_tickOnce_SavesStateEvery30Ticks(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("TICK8", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.TickCount = 29
	r.tickOnce(time.Now())
	tickCount := r.state.TickCount
	r.mu.Unlock()
	if tickCount != 30 {
		t.Fatalf("TickCount = %d, want 30", tickCount)
	}
	time.Sleep(200 * time.Millisecond)
	r.flushPersistSync()
	if repo.saveCount == 0 {
		t.Fatal("expected saveState at tick 30")
	}
}

func TestRoom_startTick_Idempotent(t *testing.T) {
	r := NewRoom("ST", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.startTick()
	if r.tickCancel == nil {
		t.Fatal("expected tick running")
	}
	r.startTick()
	if r.tickCancel == nil {
		t.Fatal("startTick should not stop active tick")
	}
	r.stopTick()
}

func TestRoom_HandleMessage_RateLimitDisconnect(t *testing.T) {
	r := NewRoom("RL", nil, nil, config.DefaultTimeoutConfig(), 0)

	server := testutil.NewWSTestUpgraderServer(t)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID: "p1", MessageCount: domain.MessageRateLimit,
		MessageWindowStart: time.Now().UnixMilli(),
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4), Conn: conn}
	r.mu.Unlock()

	_ = r.HandleMessage("p1", protocol.MsgPing, nil)
	r.mu.RLock()
	_, exists := r.connections["p1"]
	r.mu.RUnlock()
	if exists {
		t.Fatal("rate-limited player connection should be removed")
	}
}

func TestRoom_handleSetNicknameMsg_InvalidPayload(t *testing.T) {
	r := NewRoom("INV", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "Old"}
	r.mu.Lock()
	r.handleSetNicknameMsg(player, []byte{0})
	r.mu.Unlock()
	if player.NicknameConfirmed {
		t.Fatal("invalid payload should not confirm nickname")
	}
}

func TestRoom_handleSetNicknameMsg_RejectedNickname(t *testing.T) {
	r := NewRoom("REJ", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "Old"}
	r.mu.Lock()
	r.handleSetNicknameMsg(player, []byte{0}) // invalid length prefix
	r.mu.Unlock()
	if player.NicknameConfirmed {
		t.Fatal("invalid nickname should not confirm")
	}
}

// encodeTapTestPayload helper: creates a mock tap payload for testing decodeTapPayload.

func TestRoom_handleSetNicknameMsg_EmptySanitized(t *testing.T) {
	r := NewRoom("EMP", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "Old"}
	// valid framing but nickname becomes empty after sanitize
	payload := append([]byte{byte(3)}, []byte("   ")...)
	r.mu.Lock()
	r.handleSetNicknameMsg(player, payload)
	r.mu.Unlock()
	if player.NicknameConfirmed {
		t.Fatal("whitespace-only nickname should not confirm")
	}
}

func TestRoom_handleSetNicknameMsg_AcceptsValidNickname(t *testing.T) {
	r := NewRoom("OK", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "Old"}
	payload := append([]byte{byte(len("Valid"))}, []byte("Valid")...)
	r.mu.Lock()
	r.handleSetNicknameMsg(player, payload)
	r.mu.Unlock()
	if !player.NicknameConfirmed {
		t.Fatal("valid nickname should confirm")
	}
}
