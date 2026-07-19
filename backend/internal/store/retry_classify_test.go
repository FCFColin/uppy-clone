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
)

func TestIsRetryable(t *testing.T) {
	timeoutErr := net.Error(&timeoutNetErr{})
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"pgx ErrTxCommitRollback", pgx.ErrTxCommitRollback, true},
		{"pgconn ErrConnClosed", pgconn.ErrConnClosed, true},
		{"timeout net error", timeoutErr, true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"EOF", io.EOF, true},
		{"ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"generic error", errors.New("permanent failure"), false},
		{"stub SafeToRetry", stubSafeToRetryErr{}, true},
		{"stub Timeout", stubTimeoutErr{}, true},
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

type stubSafeToRetryErr struct{}

func (stubSafeToRetryErr) Error() string     { return "safe to retry" }
func (stubSafeToRetryErr) SafeToRetry() bool { return true }

type stubTimeoutErr struct{}

func (stubTimeoutErr) Error() string   { return "timeout" }
func (stubTimeoutErr) Timeout() bool   { return true }
func (stubTimeoutErr) Temporary() bool { return true }

func TestRetryableError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{"nil returns nil", nil, true},
		{"wraps transient", syscall.ECONNRESET, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RetryableError(tt.err)
			if tt.wantNil && got != nil {
				t.Fatal("RetryableError(nil) should return nil")
			}
			if !tt.wantNil && got == nil {
				t.Fatal("RetryableError should wrap transient error")
			}
		})
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

type timeoutNetErr struct{}

func (e *timeoutNetErr) Error() string   { return "timeout" }
func (e *timeoutNetErr) Timeout() bool   { return true }
func (e *timeoutNetErr) Temporary() bool { return true }

func TestJitteredBackoff_Attempt3(t *testing.T) {
	d := JitteredBackoff(50*time.Millisecond, 3)
	if d < 400*time.Millisecond {
		t.Fatalf("backoff too small: %v", d)
	}
}
