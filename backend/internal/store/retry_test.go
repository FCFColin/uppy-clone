package store

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
)

// --- Retry backoff ---

func TestJitteredBackoff(t *testing.T) {
	base := 100 * time.Millisecond

	tests := []struct {
		name    string
		attempt int
	}{
		{name: "attempt 0", attempt: 0},
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

func TestRetryWithBackoff(t *testing.T) {
	tests := []struct {
		name         string
		fn           func(attempts *int) func(_ context.Context) error
		wantAttempts int
		wantErr      bool
	}{
		{
			name:         "succeeds immediately",
			fn:           func(a *int) func(_ context.Context) error { return func(_ context.Context) error { *a++; return nil } },
			wantAttempts: 1,
		},
		{
			name: "succeeds after retries",
			fn: func(a *int) func(_ context.Context) error {
				return func(_ context.Context) error {
					*a++
					if *a < 3 {
						return retry.RetryableError(errors.New("transient error"))
					}
					return nil
				}
			},
			wantAttempts: 3,
		},
		{
			name: "exhausts retries",
			fn: func(a *int) func(_ context.Context) error {
				return func(_ context.Context) error {
					*a++
					return retry.RetryableError(errors.New("persistent error"))
				}
			},
			wantAttempts: 4,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			err := retry.Do(context.Background(), DefaultDBRetry(), tt.fn(&attempts))
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if attempts != tt.wantAttempts {
				t.Fatalf("attempts = %d, want %d", attempts, tt.wantAttempts)
			}
		})
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

// --- Error classification ---

func TestIsRetryable(t *testing.T) {
	timeoutErr := net.Error(&timeoutNetErr{})
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"pgx ErrTxCommitRollback", pgx.ErrTxCommitRollback, true},
		{"timeout net error", timeoutErr, true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"EOF", io.EOF, true},
		{"generic error", errors.New("permanent failure"), false},
		{"stub SafeToRetry", stubSafeToRetryErr{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryable(tt.err); got != tt.want {
				t.Fatalf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsRetryable_ContextDeadlineExceeded(t *testing.T) {
	orig := pgconnTimeout
	pgconnTimeout = func(err error) bool { return errors.Is(err, context.DeadlineExceeded) }
	t.Cleanup(func() { pgconnTimeout = orig })

	if !isRetryable(context.DeadlineExceeded) {
		t.Fatal("context deadline exceeded should be retryable via pgconn.Timeout")
	}
}

func TestIsRetryable_PgconnSafeToRetry(t *testing.T) {
	err := &pgconn.ConnectError{}
	if !pgconn.SafeToRetry(err) {
		t.Skip("ConnectError not marked SafeToRetry in this pgconn version")
	}
	if !isRetryable(err) {
		t.Fatal("SafeToRetry ConnectError should be retryable")
	}
}

func TestMaybeRetryable(t *testing.T) {
	persistent := errors.New("nope")
	tests := []struct {
		name    string
		err     error
		wantNil bool
		wantIs  error // for non-nil case, errors.Is check
	}{
		{"nil returns nil", nil, true, nil},
		{"transient wrapped", syscall.ECONNRESET, false, nil},
		{"persistent passed through", persistent, false, persistent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaybeRetryable(tt.err)
			if tt.wantNil {
				if got != nil {
					t.Fatal("MaybeRetryable(nil) should return nil")
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil error")
			}
			if tt.wantIs != nil && !errors.Is(got, tt.wantIs) {
				t.Fatalf("MaybeRetryable = %v", got)
			}
		})
	}
}

// --- Stub error types for classification tests ---

type stubSafeToRetryErr struct{}

func (stubSafeToRetryErr) Error() string     { return "safe to retry" }
func (stubSafeToRetryErr) SafeToRetry() bool { return true }

type timeoutNetErr struct{}

func (e *timeoutNetErr) Error() string   { return "timeout" }
func (e *timeoutNetErr) Timeout() bool   { return true }
func (e *timeoutNetErr) Temporary() bool { return true }

// --- Circuit breakers ---

// TestBreaker_FailedExecution verifies that breaker execution surfaces the
// wrapped function's error.
func TestBreaker_FailedExecution(t *testing.T) {
	cb := NewPostgresBreaker()
	_, err := cb.Execute(func() (any, error) {
		return nil, errCBTest
	})
	if err == nil {
		t.Fatal("expected error from failed execution")
	}
}

// TestBreaker_OpensAfterFailures verifies that each breaker opens after its
// configured consecutive-failure threshold.
func TestBreaker_OpensAfterFailures(t *testing.T) {
	tests := []struct {
		name     string
		breaker  func() *gobreaker.CircuitBreaker[any]
		failures int
	}{
		{"postgres", NewPostgresBreaker, 6},
		{"redis", NewRedisBreaker, 6},
		{"resend", NewResendBreaker, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetBreakersForTesting()
			cb := tt.breaker()
			for i := 0; i < tt.failures; i++ {
				_, _ = cb.Execute(func() (any, error) {
					return nil, errCBTest
				})
			}
			if cb.State() != gobreaker.StateOpen {
				t.Fatalf("breaker should be open after %d consecutive failures, got %v", tt.failures, cb.State())
			}
		})
	}
}

var errCBTest = &cbTestError{}

type cbTestError struct{}

func (e *cbTestError) Error() string { return "test error" }


