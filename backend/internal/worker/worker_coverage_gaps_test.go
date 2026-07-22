package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestEmailWorker_Start_MultipleMessages(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "email:queue",
			Values: map[string]interface{}{
				"payload": makePayload("user@test.com", "Batch", "body"),
			},
		}).Result(); err != nil {
			t.Fatalf("XAdd: %v", err)
		}
	}
	_ = rdb.XGroupCreateMkStream(ctx, "email:queue", "email-workers", "0").Err()

	var count int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	workerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(workerCtx)
		close(done)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&count) >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if atomic.LoadInt32(&count) < 3 {
		t.Fatalf("expected 3 sends, got %d", atomic.LoadInt32(&count))
	}

	cancel()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("Start did not exit")
	}
}

func TestSendEmail_JSONMarshalError(t *testing.T) {
	orig := emailJSONMarshal
	emailJSONMarshal = func(_ interface{}) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	t.Cleanup(func() { emailJSONMarshal = orig })

	w := newTestWorker(nil, "http://127.0.0.1:1")
	err := w.sendEmail(context.Background(), EmailPayload{To: "a@b.com", Subject: "s", Body: "b"})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

// ─── handleRetry tests (v2-R-85~94: outbox consumer retry + dead-letter) ──────

func TestHandleRetry_DeadLetterPath(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	// retryCount == maxRetries → dead-letter path
	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": "test", "retry_count": "3"},
	}

	moved := handleRetry(ctx, rdb, msg, "src", "grp", "dl", 3)
	if !moved {
		t.Fatal("expected dead-letter move when retryCount >= maxRetries")
	}

	// Verify message moved to dead-letter stream
	dlMsgs, err := rdb.XRange(ctx, "dl", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange dl: %v", err)
	}
	if len(dlMsgs) != 1 {
		t.Fatalf("dead-letter stream has %d messages, want 1", len(dlMsgs))
	}
	if dlMsgs[0].Values["payload"] != "test" {
		t.Errorf("dl payload = %v, want test", dlMsgs[0].Values["payload"])
	}
}

func TestHandleRetry_RetryPath(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	// retryCount < maxRetries → retry path with backoff
	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": "test", "retry_count": "0"},
	}

	moved := handleRetry(ctx, rdb, msg, "src", "grp", "dl", 3)
	if moved {
		t.Fatal("expected retry path (not dead-letter) when retryCount < maxRetries")
	}

	// Verify re-enqueued with incremented retry_count
	srcMsgs, err := rdb.XRange(ctx, "src", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange src: %v", err)
	}
	if len(srcMsgs) != 1 {
		t.Fatalf("source stream has %d messages, want 1", len(srcMsgs))
	}
	if srcMsgs[0].Values["retry_count"] != "1" {
		t.Errorf("retry_count = %v, want 1", srcMsgs[0].Values["retry_count"])
	}
	if srcMsgs[0].Values["payload"] != "test" {
		t.Errorf("payload = %v, want test", srcMsgs[0].Values["payload"])
	}
}

func TestHandleRetry_ContextCancellation(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately to trigger backoff abort

	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": "test", "retry_count": "0"},
	}

	moved := handleRetry(ctx, rdb, msg, "src", "grp", "dl", 3)
	if moved {
		t.Fatal("expected false on context cancellation")
	}

	// Verify no re-enqueue happened
	srcMsgs, _ := rdb.XRange(context.Background(), "src", "-", "+").Result()
	if len(srcMsgs) != 0 {
		t.Fatalf("source stream has %d messages, want 0", len(srcMsgs))
	}
}

func TestHandleRetry_DeadLetterXAddError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		mr.Close()
		_ = rdb.Close()
	})
	ctx := context.Background()

	// Close miniredis to cause XAdd error
	mr.Close()

	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": "test", "retry_count": "5"},
	}

	// Should return false when XAdd to dead-letter fails (message remains in PEL)
	moved := handleRetry(ctx, rdb, msg, "src", "grp", "dl", 3)
	if moved {
		t.Fatal("expected false when dead-letter XAdd fails — message should stay in PEL")
	}
}

func TestHandleRetry_NoRetryCount(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	// No retry_count field → treated as 0 → retry path
	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": "fresh"},
	}

	moved := handleRetry(ctx, rdb, msg, "src", "grp", "dl", 3)
	if moved {
		t.Fatal("expected retry path for message without retry_count")
	}

	srcMsgs, _ := rdb.XRange(ctx, "src", "-", "+").Result()
	if len(srcMsgs) != 1 {
		t.Fatalf("source stream has %d messages, want 1", len(srcMsgs))
	}
	if srcMsgs[0].Values["retry_count"] != "1" {
		t.Errorf("retry_count = %v, want 1", srcMsgs[0].Values["retry_count"])
	}
}
