package resilience

import (
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sethvargo/go-retry"
)

// DefaultDBRetry returns a backoff for database operations (3 retries, 100ms base).
func DefaultDBRetry() retry.Backoff {
	b := retry.NewExponential(100 * time.Millisecond)
	return retry.WithMaxRetries(3, retry.WithJitter(50*time.Millisecond, b))
}

// DefaultRedisRetry returns a backoff for Redis operations (2 retries, 50ms base).
func DefaultRedisRetry() retry.Backoff {
	b := retry.NewExponential(50 * time.Millisecond)
	return retry.WithMaxRetries(2, retry.WithJitter(25*time.Millisecond, b))
}

// ExternalAPIRetry returns a backoff for external API calls (2 retries, 500ms base).
func ExternalAPIRetry() retry.Backoff {
	b := retry.NewExponential(500 * time.Millisecond)
	return retry.WithMaxRetries(2, retry.WithJitter(200*time.Millisecond, b))
}

// JitteredBackoff computes an exponential backoff with jitter for the given attempt.
func JitteredBackoff(base time.Duration, attempt int) time.Duration {
	backoff := base * time.Duration(1<<uint(attempt))        //nolint:gosec // G115: attempt bounded by retry count (max 3)
	jitter := time.Duration(rand.Int64N(int64(backoff) / 2)) //nolint:gosec // G404: retry jitter, not crypto
	return backoff + jitter
}

var pgconnTimeout = pgconn.Timeout

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return true
	}
	if errors.Is(err, pgconn.ErrConnClosed) {
		return true
	}
	if pgconn.SafeToRetry(err) {
		return true
	}
	if pgconnTimeout(err) {
		return true
	}
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

// RetryableError wraps err as a retryable error for the retry package.
func RetryableError(err error) error {
	if err == nil {
		return nil
	}
	return retry.RetryableError(err)
}

// MaybeRetryable wraps err as retryable when isRetryable reports it as transient.
func MaybeRetryable(err error) error {
	if err == nil {
		return nil
	}
	if isRetryable(err) {
		return retry.RetryableError(err)
	}
	return err
}

// IsHTTPRetryable classifies HTTP response status codes for retry decisions (audit-017).
// 429 (Too Many Requests) and 5xx (Server Error) are retryable.
// 4xx (Client Error, except 429/408) are permanent failures and should not be retried.
// 408 (Request Timeout) is retryable.
func IsHTTPRetryable(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusRequestTimeout {
		return true
	}
	if statusCode >= 500 && statusCode < 600 {
		return true
	}
	return false
}
