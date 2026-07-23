package game

import (
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

	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:                 "p1",
		Nickname:           "TestPlayer",
		MessageCount:       domain.MessageRateLimit - 1,
		MessageWindowStart: time.Now().UnixMilli(),
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.mu.Unlock()

	err := r.HandleMessage("p1", protocol.MsgPing, nil)
	if err != nil {
		t.Fatalf("HandleMessage should not error, got %v", err)
	}

	r.mu.RLock()
	count := r.state.Players["p1"].MessageCount
	r.mu.RUnlock()
	if count != domain.MessageRateLimit {
		t.Fatalf("expected MessageCount=%d, got %d", domain.MessageRateLimit, count)
	}
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

func TestUpdatePlayerStats(t *testing.T) {
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
}

// TestRoom_tickOnce_CollisionEndsGame covers ground, ghost, and bird collisions
// that should transition phase to PhaseEnded.
func TestRoom_tickOnce_CollisionEndsGame(t *testing.T) {
	cases := []struct {
		name  string
		setup func(state *domain.GameState)
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

func TestRoom_HandleTap(t *testing.T) {
	cases := []struct {
		name          string
		phase         domain.GamePhase
		cooldownEnd   int64
		payload       []byte
		wantMsg       byte
		wantScoreIncr bool
	}{
		{
			name:          "AcceptsValidTap",
			phase:         domain.PhasePlaying,
			payload:       encodeTapTestPayload(0.5, 0.5),
			wantMsg:       protocol.MsgTapAccepted,
			wantScoreIncr: true,
		},
		{
			name:    "RejectsWrongPhase",
			phase:   domain.PhaseWaiting,
			payload: encodeTapTestPayload(0.5, 0.5),
			wantMsg: protocol.MsgTapRejected,
		},
		{
			name:        "RejectsCooldown",
			phase:       domain.PhasePlaying,
			cooldownEnd: time.Now().UnixMilli() + 5000,
			payload:     encodeTapTestPayload(0.5, 0.5),
			wantMsg:     protocol.MsgTapRejected,
		},
		{
			name:    "RejectsInvalidPayload",
			phase:   domain.PhasePlaying,
			payload: []byte{0x01},
			wantMsg: protocol.MsgTapRejected,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			timeouts := config.DefaultTimeoutConfig()
			r := NewRoom("TEST1", nil, nil, timeouts, 0)
			r.syncOutbound = true
			r.state.Phase = c.phase
			r.state.Balloon.X = 0.5
			r.state.Balloon.Y = 0.5

			player := &domain.PlayerState{ID: "p1", PlayerIndex: 0, CooldownEndTime: c.cooldownEnd}
			ch := make(chan []byte, 64)
			r.mu.Lock()
			r.state.Players["p1"] = player
			r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
			r.mu.Unlock()

			beforeScore := r.state.Balloon.Score
			r.handleTap(player, "p1", c.payload)

			if c.wantScoreIncr && r.state.Balloon.Score != beforeScore+1 {
				t.Fatalf("score = %d, want %d", r.state.Balloon.Score, beforeScore+1)
			}
			if c.wantScoreIncr && player.TapsCount != 1 {
				t.Fatalf("TapsCount = %d, want 1", player.TapsCount)
			}
			select {
			case msg := <-ch:
				if len(msg) == 0 || msg[0] != c.wantMsg {
					t.Fatalf("expected msg 0x%02x, got %v", c.wantMsg, msg)
				}
			default:
				t.Fatal("expected message on player channel")
			}
		})
	}
}
