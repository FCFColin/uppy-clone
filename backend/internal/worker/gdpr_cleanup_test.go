package worker

import (
	"context"
	"testing"
	"time"
)

type mockGDPRDeleter struct {
	deleted int64
	err     error
	calls   int
}

func (m *mockGDPRDeleter) HardDeleteExpiredUsers(_ context.Context, _ int) (int64, error) {
	m.calls++
	return m.deleted, m.err
}

func TestGDPRCleanupWorker_RunOnce_DeletesUsers(t *testing.T) {
	deleter := &mockGDPRDeleter{deleted: 2}
	w := NewGDPRCleanupWorker(nil, 30, time.Hour)
	w.hardDelete = deleter.HardDeleteExpiredUsers
	w.runOnce(context.Background())

	if deleter.calls != 1 {
		t.Fatalf("calls = %d, want 1", deleter.calls)
	}
}

func TestGDPRCleanupWorker_Start_Cancelled(t *testing.T) {
	deleter := &mockGDPRDeleter{}
	w := NewGDPRCleanupWorker(nil, 30, 10*time.Millisecond)
	w.hardDelete = deleter.HardDeleteExpiredUsers
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}
	if deleter.calls < 1 {
		t.Fatal("expected at least one cleanup run")
	}
}

func TestNewGDPRCleanupWorker(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		retentionDays int
		interval      time.Duration
		wantRetention int
		wantInterval  time.Duration
	}{
		{"defaults", 0, 0, defaultGDPRRetentionDays, defaultGDPRCleanupInterval},
		{"custom", 60, 12 * time.Hour, 60, 12 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewGDPRCleanupWorker(nil, tc.retentionDays, tc.interval)
			if w.retentionDays != tc.wantRetention {
				t.Errorf("retentionDays = %d, want %d", w.retentionDays, tc.wantRetention)
			}
			if w.interval != tc.wantInterval {
				t.Errorf("interval = %v, want %v", w.interval, tc.wantInterval)
			}
		})
	}
}
