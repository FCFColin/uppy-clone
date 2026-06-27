package game

import (
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

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
