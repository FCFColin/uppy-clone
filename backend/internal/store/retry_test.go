package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sethvargo/go-retry"
)

func TestDefaultDBRetry(t *testing.T) {
	b := DefaultDBRetry()
	if b == nil {
		t.Fatal("DefaultDBRetry returned nil")
	}
}

func TestDefaultRedisRetry(t *testing.T) {
	b := DefaultRedisRetry()
	if b == nil {
		t.Fatal("DefaultRedisRetry returned nil")
	}
}

func TestJitteredBackoff(t *testing.T) {
	base := 100 * time.Millisecond

	tests := []struct {
		name    string
		attempt int
	}{
		{name: "attempt 0", attempt: 0},
		{name: "attempt 1", attempt: 1},
		{name: "attempt 2", attempt: 2},
		{name: "attempt 5", attempt: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := JitteredBackoff(base, tt.attempt)
			minExpected := base * time.Duration(1<<uint(tt.attempt)) //nolint:gosec // G115: test value, bounded
			if d < minExpected {
				t.Fatalf("backoff too small: got %v, minimum expected %v for attempt %d", d, minExpected, tt.attempt)
			}
			maxExpected := minExpected + minExpected/2
			if d > maxExpected {
				t.Fatalf("backoff too large: got %v, maximum expected %v for attempt %d", d, maxExpected, tt.attempt)
			}
		})
	}
}

func TestJitteredBackoff_ZeroBase(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for zero base, but JitteredBackoff did not panic")
		}
	}()
	JitteredBackoff(0, 3)
}

func TestRetryWithBackoff_SucceedsImmediately(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := retry.Do(ctx, DefaultDBRetry(), func(_ context.Context) error {
		attempts++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryWithBackoff_SucceedsAfterRetries(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := retry.Do(ctx, DefaultDBRetry(), func(_ context.Context) error {
		attempts++
		if attempts < 3 {
			return retry.RetryableError(errors.New("transient error"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retries, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_ExhaustsRetries(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := retry.Do(ctx, DefaultDBRetry(), func(_ context.Context) error {
		attempts++
		return retry.RetryableError(errors.New("persistent error"))
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 4 {
		t.Fatalf("expected 4 attempts (1 initial + 3 retries), got %d", attempts)
	}
}

func TestRetryWithBackoff_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := retry.Do(ctx, DefaultDBRetry(), func(_ context.Context) error {
		return errors.New("should not execute")
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func BenchmarkJitteredBackoff(b *testing.B) {
	base := 100 * time.Millisecond
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		JitteredBackoff(base, i%10)
	}
}
