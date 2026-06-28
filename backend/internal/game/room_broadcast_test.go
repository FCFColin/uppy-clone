package game

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

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

func TestBuildSnapshot_SkipsDisconnectedPlayers(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST"),
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.state.Players["p1"] = &domain.PlayerState{Nickname: "Alice", PlayerIndex: 0}
	room.state.Players["p2"] = &domain.PlayerState{Nickname: "Bob", PlayerIndex: 1, Disconnected: true}
	room.buildSnapshot()
	if len(room.players) != 1 {
		t.Fatalf("connected players in snapshot = %d, want 1", len(room.players))
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
