package domain

import (
	"math"
	"testing"
	"time"
)

func TestPlayerCanTap(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()

	t.Run("can tap when cooldown expired", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: now - 1}
		if !p.CanTap(now) {
			t.Error("CanTap should return true when cooldown has expired")
		}
	})

	t.Run("cannot tap during cooldown", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: now + 10000}
		if p.CanTap(now) {
			t.Error("CanTap should return false during cooldown")
		}
	})

	t.Run("can tap when cooldown equals now", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: now}
		if !p.CanTap(now) {
			t.Error("CanTap should return true when cooldown equals now")
		}
	})

	t.Run("can tap with zero cooldown", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: 0}
		if !p.CanTap(now) {
			t.Error("CanTap should return true with zero cooldown")
		}
	})
}

func TestPlayerRecordTap(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()

	p := &PlayerState{CooldownEndTime: 0, TapsCount: 0, ScoreContribution: 0}
	p.RecordTap(now, 5000)

	if p.CooldownEndTime != now+5000 {
		t.Errorf("CooldownEndTime = %d, want %d", p.CooldownEndTime, now+5000)
	}
	if p.TapsCount != 1 {
		t.Errorf("TapsCount = %d, want 1", p.TapsCount)
	}
	if p.ScoreContribution != 1 {
		t.Errorf("ScoreContribution = %d, want 1", p.ScoreContribution)
	}
}

func TestPlayerRecordTap_Multiple(t *testing.T) {
	t.Parallel()
	p := &PlayerState{}
	for i := 0; i < 5; i++ {
		p.RecordTap(1000, 100)
	}
	if p.TapsCount != 5 {
		t.Errorf("TapsCount = %d, want 5", p.TapsCount)
	}
	if p.ScoreContribution != 5 {
		t.Errorf("ScoreContribution = %d, want 5", p.ScoreContribution)
	}
}

func TestPlayerIsRateLimited(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()
	const windowMs = 1000
	const maxMessages = 5

	t.Run("not limited within window under max", func(t *testing.T) {
		p := &PlayerState{MessageWindowStart: now, MessageCount: 3}
		if p.IsRateLimited(now, windowMs, maxMessages) {
			t.Error("Should not be rate limited with count under max")
		}
	})

	t.Run("limited when over max within window", func(t *testing.T) {
		p := &PlayerState{MessageWindowStart: now, MessageCount: 10}
		if !p.IsRateLimited(now, windowMs, maxMessages) {
			t.Error("Should be rate limited when over max within window")
		}
	})

	t.Run("not limited when window expired regardless of count", func(t *testing.T) {
		p := &PlayerState{MessageWindowStart: now - windowMs - 1, MessageCount: 100}
		if p.IsRateLimited(now, windowMs, maxMessages) {
			t.Error("Should not be rate limited when window has expired")
		}
	})

	t.Run("exactly at boundary: not limited", func(t *testing.T) {
		p := &PlayerState{MessageWindowStart: now, MessageCount: 5}
		if p.IsRateLimited(now, windowMs, maxMessages) {
			t.Error("Should not be rate limited when count equals max")
		}
	})

	t.Run("zero max messages always limited", func(t *testing.T) {
		p := &PlayerState{MessageWindowStart: now, MessageCount: 1}
		if !p.IsRateLimited(now, windowMs, 0) {
			t.Error("Should be rate limited when maxMessages is 0")
		}
	})
}

func TestPlayerMarkDisconnected(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()

	p := &PlayerState{Disconnected: false, DisconnectedAt: nil}
	p.MarkDisconnected(now)

	if !p.Disconnected {
		t.Error("Disconnected should be true after MarkDisconnected")
	}
	if p.DisconnectedAt == nil {
		t.Fatal("DisconnectedAt should not be nil")
	}
	if *p.DisconnectedAt != now {
		t.Errorf("DisconnectedAt = %d, want %d", *p.DisconnectedAt, now)
	}
}

func TestPlayerReconnect(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()

	p := &PlayerState{Disconnected: true, DisconnectedAt: &now}
	p.Reconnect()

	if p.Disconnected {
		t.Error("Disconnected should be false after Reconnect")
	}
	if p.DisconnectedAt != nil {
		t.Error("DisconnectedAt should be nil after Reconnect")
	}
}

func TestGameStateAddPlayer(t *testing.T) {
	t.Parallel()
	gs := &GameState{Players: make(map[string]*PlayerState)}
	p := &PlayerState{ID: "p1", Nickname: "Alice"}
	if err := gs.AddPlayer(p); err != nil {
		t.Fatalf("AddPlayer returned error: %v", err)
	}

	if _, ok := gs.Players["p1"]; !ok {
		t.Error("AddPlayer should add player to Players map")
	}
	if gs.Players["p1"].Nickname != "Alice" {
		t.Errorf("Nickname = %q, want %q", gs.Players["p1"].Nickname, "Alice")
	}
}

func TestGameStateRemovePlayer(t *testing.T) {
	t.Parallel()
	gs := &GameState{Players: map[string]*PlayerState{
		"p1": {ID: "p1"},
		"p2": {ID: "p2"},
	}}
	gs.RemovePlayer("p1")

	if _, ok := gs.Players["p1"]; ok {
		t.Error("RemovePlayer should remove player")
	}
	if _, ok := gs.Players["p2"]; !ok {
		t.Error("RemovePlayer should not affect other players")
	}
}

func TestGameStateRemovePlayer_NotFound(t *testing.T) {
	t.Parallel()
	gs := &GameState{Players: make(map[string]*PlayerState)}
	gs.RemovePlayer("nonexistent")
	// Must not panic
}

func TestGameStateRemovePlayer_NilMap(_ *testing.T) {
	gs := &GameState{}
	gs.RemovePlayer("p1")
	// Must not panic
}

func TestGameStateUpdatePlayerState(t *testing.T) {
	t.Parallel()
	gs := &GameState{Players: map[string]*PlayerState{
		"p1": {Nickname: "old", NicknameConfirmed: false},
	}}
	gs.UpdatePlayerState("p1", func(p *PlayerState) {
		p.NicknameConfirmed = true
	})

	if !gs.Players["p1"].NicknameConfirmed {
		t.Error("UpdatePlayerState should apply the update function")
	}
}

func TestGameStateUpdatePlayerState_NotFound(t *testing.T) {
	t.Parallel()
	gs := &GameState{Players: make(map[string]*PlayerState)}
	called := false
	gs.UpdatePlayerState("nonexistent", func(_ *PlayerState) {
		called = true
	})
	if called {
		t.Error("UpdatePlayerState should not call function for nonexistent player")
	}
}

func TestGameStateUpdatePlayerState_NilMap(t *testing.T) {
	gs := &GameState{}
	called := false
	gs.UpdatePlayerState("p1", func(_ *PlayerState) {
		called = true
	})
	if called {
		t.Error("UpdatePlayerState should not call function when Players is nil")
	}
}

func TestGameStateIsGameOver(t *testing.T) {
	t.Parallel()

	t.Run("ended phase returns true", func(t *testing.T) {
		gs := &GameState{Phase: PhaseEnded}
		if !gs.IsGameOver() {
			t.Error("IsGameOver should return true for PhaseEnded")
		}
	})

	t.Run("waiting phase returns false", func(t *testing.T) {
		gs := &GameState{Phase: PhaseWaiting}
		if gs.IsGameOver() {
			t.Error("IsGameOver should return false for PhaseWaiting")
		}
	})

	t.Run("countdown phase returns false", func(t *testing.T) {
		gs := &GameState{Phase: PhaseCountdown}
		if gs.IsGameOver() {
			t.Error("IsGameOver should return false for PhaseCountdown")
		}
	})

	t.Run("playing phase returns false", func(t *testing.T) {
		gs := &GameState{Phase: PhasePlaying}
		if gs.IsGameOver() {
			t.Error("IsGameOver should return false for PhasePlaying")
		}
	})
}

func TestPlayerState_RateLimitEdgeNowZero(t *testing.T) {
	t.Parallel()
	// SECURITY: MessageWindowStart and now at the boundary — if window calculation
	// uses unsigned math or incorrect comparison, this could bypass rate limiting.
	p := &PlayerState{MessageWindowStart: 0, MessageCount: 100}
	if p.IsRateLimited(math.MaxInt64, 1000, 5) {
		t.Error("IsRateLimited should return false when now is far in the future")
	}
}

func TestPlayerState_RateLimitNegativeNow(t *testing.T) {
	t.Parallel()
	// SECURITY: Negative timestamps could cause window arithmetic overflow.
	// Ensure the function still produces correct results.
	p := &PlayerState{MessageWindowStart: -5000, MessageCount: 100}
	if p.IsRateLimited(-1000, 1000, 5) {
		t.Error("IsRateLimited with negative timestamps should not break")
	}
}
