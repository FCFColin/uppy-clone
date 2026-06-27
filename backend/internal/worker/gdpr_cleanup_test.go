package worker

import (
	"testing"
	"time"
)

func TestNewGDPRCleanupWorker_Defaults(t *testing.T) {
	t.Parallel()
	w := NewGDPRCleanupWorker(nil, 0, 0)
	if w.retentionDays != defaultGDPRRetentionDays {
		t.Errorf("retentionDays = %d, want %d", w.retentionDays, defaultGDPRRetentionDays)
	}
	if w.interval != defaultGDPRCleanupInterval {
		t.Errorf("interval = %v, want %v", w.interval, defaultGDPRCleanupInterval)
	}
}

func TestNewGDPRCleanupWorker_NegativeRetention(t *testing.T) {
	t.Parallel()
	w := NewGDPRCleanupWorker(nil, -5, 0)
	if w.retentionDays != defaultGDPRRetentionDays {
		t.Errorf("retentionDays = %d, want %d", w.retentionDays, defaultGDPRRetentionDays)
	}
}

func TestNewGDPRCleanupWorker_NegativeInterval(t *testing.T) {
	t.Parallel()
	w := NewGDPRCleanupWorker(nil, 0, -1*time.Hour)
	if w.interval != defaultGDPRCleanupInterval {
		t.Errorf("interval = %v, want %v", w.interval, defaultGDPRCleanupInterval)
	}
}

func TestNewGDPRCleanupWorker_CustomValues(t *testing.T) {
	t.Parallel()
	w := NewGDPRCleanupWorker(nil, 60, 12*time.Hour)
	if w.retentionDays != 60 {
		t.Errorf("retentionDays = %d, want 60", w.retentionDays)
	}
	if w.interval != 12*time.Hour {
		t.Errorf("interval = %v, want 12h", w.interval)
	}
}

func TestNewGDPRCleanupWorker_NilDB(t *testing.T) {
	w := NewGDPRCleanupWorker(nil, 30, 24*time.Hour)
	if w.db != nil {
		t.Error("db should be nil when nil is passed")
	}
}
