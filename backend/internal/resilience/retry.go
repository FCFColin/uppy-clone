package resilience

import (
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sethvargo/go-retry"
)

// Enterprise rationale: Transient failures (network blips, temporary overload)
// are normal in distributed systems. Retry with backoff handles them automatically.
// Jitter prevents thundering herd — all clients retrying simultaneously after
// a shared failure. Trade-off: Retries add latency for failed requests.
// Non-idempotent operations (e.g., payment) must NOT be retried blindly.

// DefaultDBRetry is a retry policy for database operations (idempotent reads).
func DefaultDBRetry() retry.Backoff {
	b := retry.NewExponential(100 * time.Millisecond)
	return retry.WithMaxRetries(3, retry.WithJitter(50*time.Millisecond, b))
}

// DefaultRedisRetry is a retry policy for Redis operations.
func DefaultRedisRetry() retry.Backoff {
	b := retry.NewExponential(50 * time.Millisecond)
	return retry.WithMaxRetries(2, retry.WithJitter(25*time.Millisecond, b))
}

// ExternalAPIRetry is a retry policy for external API calls (e.g., Resend).
// More conservative: longer backoff, fewer retries.
func ExternalAPIRetry() retry.Backoff {
	b := retry.NewExponential(500 * time.Millisecond)
	return retry.WithMaxRetries(2, retry.WithJitter(200*time.Millisecond, b))
}

// JitteredBackoff returns a backoff duration with random jitter to prevent
// thundering herd effect. This is a standalone helper for manual retry loops.
func JitteredBackoff(base time.Duration, attempt int) time.Duration {
	backoff := base * time.Duration(1<<uint(attempt))        //nolint:gosec // attempt is bounded by maxRetries
	jitter := time.Duration(rand.Int64N(int64(backoff) / 2)) //nolint:gosec // jitter uses math/rand intentionally, not security-sensitive
	return backoff + jitter
}

// isRetryable reports whether an error represents a transient failure that
// is safe to retry for idempotent operations.
//
// Enterprise rationale: sethvargo/go-retry only retries errors wrapped with
// retry.RetryableError; returning a plain error causes retry.Do to stop
// immediately. Without this classification, configured retries never execute
// and transient faults (network blips, connection resets) surface directly
// to users as 5xx errors.
//
// Classification is conservative: only errors known to be transient are
// retryable. Persistent errors (constraint violations, not-found, permission
// denied) return false so the retry loop aborts immediately.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// pgx/pgconn transient errors
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return true
	}
	if errors.Is(err, pgconn.ErrConnClosed) {
		return true
	}
	// pgconn.SafeToRetry reports whether the pgconn library considers the error
	// safe to retry (e.g., connection-level errors where the request was not sent
	// or the server rolled back). This is the canonical pgx v5 retry classification.
	if pgconn.SafeToRetry(err) {
		return true
	}
	// pgconn.Timeout reports whether the error is a timeout (query/connect).
	if pgconn.Timeout(err) {
		return true
	}

	// Network transient errors
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	return false
}

// RetryableError wraps err as a retryable error for sethvargo/go-retry.
// Returns nil if err is nil. Use this in retry.Do callbacks:
//
//	if isRetryable(err) { return retry.RetryableError(err) }
//	return err
func RetryableError(err error) error {
	if err == nil {
		return nil
	}
	return retry.RetryableError(err)
}

// MaybeRetryable wraps err with RetryableError if isRetryable(err) is true,
// otherwise returns err unchanged. This is the recommended helper for
// retry.Do callbacks that want to retry transient errors while failing
// fast on persistent errors.
func MaybeRetryable(err error) error {
	if err == nil {
		return nil
	}
	if isRetryable(err) {
		return retry.RetryableError(err)
	}
	return err
}
