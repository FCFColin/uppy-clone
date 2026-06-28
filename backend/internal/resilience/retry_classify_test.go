package resilience

import (
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryable_Nil(t *testing.T) {
	if isRetryable(nil) {
		t.Fatal("nil error should not be retryable")
	}
}

func TestIsRetryable_PgxErrors(t *testing.T) {
	if !isRetryable(pgx.ErrTxCommitRollback) {
		t.Fatal("ErrTxCommitRollback should be retryable")
	}
	if !isRetryable(pgconn.ErrConnClosed) {
		t.Fatal("ErrConnClosed should be retryable")
	}
}

func TestIsRetryable_NetworkErrors(t *testing.T) {
	timeoutErr := net.Error(&timeoutNetErr{})
	if !isRetryable(timeoutErr) {
		t.Fatal("timeout net error should be retryable")
	}
	if !isRetryable(syscall.ECONNRESET) {
		t.Fatal("ECONNRESET should be retryable")
	}
	if !isRetryable(syscall.ECONNREFUSED) {
		t.Fatal("ECONNREFUSED should be retryable")
	}
	if !isRetryable(io.EOF) {
		t.Fatal("EOF should be retryable")
	}
	if !isRetryable(io.ErrUnexpectedEOF) {
		t.Fatal("ErrUnexpectedEOF should be retryable")
	}
}

func TestIsRetryable_PersistentError(t *testing.T) {
	if isRetryable(errors.New("permanent failure")) {
		t.Fatal("generic error should not be retryable")
	}
}

func TestRetryableError_Nil(t *testing.T) {
	if RetryableError(nil) != nil {
		t.Fatal("RetryableError(nil) should return nil")
	}
}

func TestMaybeRetryable_Transient(t *testing.T) {
	err := MaybeRetryable(syscall.ECONNRESET)
	if err == nil {
		t.Fatal("expected retryable error wrapper")
	}
}

func TestMaybeRetryable_Persistent(t *testing.T) {
	orig := errors.New("nope")
	if got := MaybeRetryable(orig); !errors.Is(got, orig) {
		t.Fatalf("MaybeRetryable = %v", got)
	}
}

type timeoutNetErr struct{}

func (e *timeoutNetErr) Error() string   { return "timeout" }
func (e *timeoutNetErr) Timeout() bool   { return true }
func (e *timeoutNetErr) Temporary() bool { return true }

func TestMaybeRetryable_Nil(t *testing.T) {
	if MaybeRetryable(nil) != nil {
		t.Fatal("MaybeRetryable(nil) should return nil")
	}
}

func TestJitteredBackoff_Attempt3(t *testing.T) {
	d := JitteredBackoff(50*time.Millisecond, 3)
	if d < 400*time.Millisecond {
		t.Fatalf("backoff too small: %v", d)
	}
}
