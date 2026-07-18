package worker

import (
	"context"
	"errors"
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
	w := NewGDPRCleanupWorker(deleter, 30, time.Hour)
	w.runOnce(context.Background())

	if deleter.calls != 1 {
		t.Fatalf("calls = %d, want 1", deleter.calls)
	}
}

func TestGDPRCleanupWorker_RunOnce_Error(t *testing.T) {
	deleter := &mockGDPRDeleter{err: errors.New("delete failed")}
	w := NewGDPRCleanupWorker(deleter, 30, time.Hour)
	w.runOnce(context.Background())
	if deleter.calls != 1 {
		t.Fatal("expected cleanup attempt")
	}
}

func TestGDPRCleanupWorker_Start_Cancelled(t *testing.T) {
	deleter := &mockGDPRDeleter{}
	w := NewGDPRCleanupWorker(deleter, 30, 10*time.Millisecond)
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
