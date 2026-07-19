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

// TestRoom_tickOnce_CollisionEndsGame covers ground, ghost, and bird collisions
// that should transition phase to PhaseEnded.
func TestRoom_tickOnce_CollisionEndsGame(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(state *domain.GameState)
	}{
		{
			name: "GameOverGround",
			setup: func(s *domain.GameState) {
				s.Balloon.Y = 0.001
				s.Balloon.VY = -0.1
			},
		},
		{
			name: "GhostCollision",
			setup: func(s *domain.GameState) {
				s.Ghost.Active = true
				s.Ghost.X = s.Balloon.X
				s.Ghost.Y = s.Balloon.Y
				s.Ghost.VX = 0
				s.Ghost.VY = 0
				s.Ghost.RepelTimer = 0
			},
		},
		{
			name: "BirdCollision",
			setup: func(s *domain.GameState) {
				s.Bird.Active = true
				s.Bird.X = s.Balloon.X
				s.Bird.Y = s.Balloon.Y
				s.Bird.VX = 0
				s.Bird.VY = 0
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewRoom("TICK", nil, nil, config.DefaultTimeoutConfig(), 0)
			r.syncOutbound = true
			addConnectedPlayer(r, "p1")
			r.mu.Lock()
			r.state.Phase = domain.PhasePlaying
			c.setup(r.state)
			r.tickOnce(time.Now())
			phase := r.state.Phase
			r.mu.Unlock()
			if phase != domain.PhaseEnded {
				t.Fatalf("phase = %s, want ended after %s", phase, c.name)
			}
		})
	}
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

// TestRoom_tickOnce_StopsWhenNoActivePlayers covers both all-disconnected and zero-players cases.
func TestRoom_tickOnce_StopsWhenNoActivePlayers(t *testing.T) {
	cases := []struct {
		name  string
		setup func(state *domain.GameState)
	}{
		{
			name: "AllDisconnected",
			setup: func(s *domain.GameState) {
				now := time.Now().UnixMilli()
				s.Players["p1"] = &domain.PlayerState{ID: "p1", Disconnected: true, DisconnectedAt: &now}
			},
		},
		{
			name:  "EmptyPlayers",
			setup: func(_ *domain.GameState) {},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewRoom("TICK", nil, nil, config.DefaultTimeoutConfig(), 0)
			r.mu.Lock()
			r.state.Phase = domain.PhasePlaying
			c.setup(r.state)
			r.startTick()
			if r.tickCancel == nil {
				t.Fatal("expected tick to start")
			}
			r.tickOnce(time.Now())
			if r.tickCancel != nil {
				t.Fatal("expected tick to stop with no active players")
			}
			r.mu.Unlock()
		})
	}
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

func TestRoom_handleSetNicknameMsg(t *testing.T) {
	cases := []struct {
		name             string
		payload          []byte
		wantConfirmed    bool
	}{
		{name: "InvalidPayloadZero", payload: []byte{0}, wantConfirmed: false},
		{name: "EmptySanitized", payload: append([]byte{byte(3)}, []byte("   ")...), wantConfirmed: false},
		{name: "AcceptsValidNickname", payload: append([]byte{byte(len("Valid"))}, []byte("Valid")...), wantConfirmed: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewRoom("NICK", nil, nil, config.DefaultTimeoutConfig(), 0)
			player := &domain.PlayerState{ID: "p1", Nickname: "Old"}
			r.mu.Lock()
			r.handleSetNicknameMsg(player, c.payload)
			r.mu.Unlock()
			if player.NicknameConfirmed != c.wantConfirmed {
				t.Fatalf("NicknameConfirmed = %v, want %v", player.NicknameConfirmed, c.wantConfirmed)
			}
		})
	}
}

// encodeTapTestPayload helper: creates a mock tap payload for testing decodeTapPayload.
