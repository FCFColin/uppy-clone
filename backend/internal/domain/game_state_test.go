package domain

import (
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
