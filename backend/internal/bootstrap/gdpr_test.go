package bootstrap

import (
	"testing"
	"time"
)

func TestResolveGDPRConfig_AppliesDefaults(t *testing.T) {
	t.Parallel()
	days, interval := ResolveGDPRConfig(0, 0)
	if days != DefaultGDPRRetentionDays {
		t.Fatalf("days = %d, want %d", days, DefaultGDPRRetentionDays)
	}
	if interval != DefaultGDPRCleanupInterval {
		t.Fatalf("interval = %v, want %v", interval, DefaultGDPRCleanupInterval)
	}
}

func TestResolveGDPRConfig_PositiveValuesPreserved(t *testing.T) {
	t.Parallel()
	days, interval := ResolveGDPRConfig(60, 12)
	if days != 60 {
		t.Fatalf("days = %d, want 60", days)
	}
	if interval != 12*time.Hour {
		t.Fatalf("interval = %v, want %v", interval, 12*time.Hour)
	}
}

func TestResolveGDPRConfig_ZeroDaysKeepsInterval(t *testing.T) {
	t.Parallel()
	days, interval := ResolveGDPRConfig(0, 24)
	if days != DefaultGDPRRetentionDays {
		t.Fatalf("days = %d, want %d", days, DefaultGDPRRetentionDays)
	}
	if interval != 24*time.Hour {
		t.Fatalf("interval = %v, want 24h", interval)
	}
}

func TestResolveGDPRConfig_ZeroIntervalKeepsDays(t *testing.T) {
	t.Parallel()
	days, interval := ResolveGDPRConfig(45, 0)
	if days != 45 {
		t.Fatalf("days = %d, want 45", days)
	}
	if interval != DefaultGDPRCleanupInterval {
		t.Fatalf("interval = %v, want %v", interval, DefaultGDPRCleanupInterval)
	}
}
