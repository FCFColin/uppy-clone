package bootstrap

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartWorker_LaunchesGoroutine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var wg sync.WaitGroup
	var called int32

	StartWorker(ctx, &wg, "test-worker", func(_ context.Context) {
		atomic.StoreInt32(&called, 1)
	})

	// Wait briefly for goroutine to run, then wait for completion
	time.Sleep(20 * time.Millisecond)
	wg.Wait()

	if atomic.LoadInt32(&called) != 1 {
		t.Fatal("worker goroutine was not called")
	}
}

func TestStartWorker_WaitsForCompletion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var wg sync.WaitGroup

	done := make(chan struct{})
	StartWorker(ctx, &wg, "blocking-worker", func(_ context.Context) {
		time.Sleep(30 * time.Millisecond)
		close(done)
	})

	// wg.Wait should block until the worker's fn returns
	wg.Wait()

	select {
	case <-done:
		// expected: worker fn completed
	default:
		t.Fatal("wg.Wait() returned but worker fn had not completed")
	}
}

func TestStartWorker_RespectsContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	cancelled := make(chan struct{})
	StartWorker(ctx, &wg, "ctx-aware-worker", func(workerCtx context.Context) {
		<-workerCtx.Done()
		close(cancelled)
	})

	cancel()
	wg.Wait()

	select {
	case <-cancelled:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("worker did not observe context cancellation")
	}
}

func TestStartWorker_MultipleWorkers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var wg sync.WaitGroup

	var counter int32
	for i := 0; i < 5; i++ {
		StartWorker(ctx, &wg, "multi-worker", func(_ context.Context) {
			atomic.AddInt32(&counter, 1)
		})
	}

	wg.Wait()
	if got := atomic.LoadInt32(&counter); got != 5 {
		t.Fatalf("counter = %d, want 5", got)
	}
}
