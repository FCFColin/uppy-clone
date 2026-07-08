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

func DefaultDBRetry() retry.Backoff {
	b := retry.NewExponential(100 * time.Millisecond)
	return retry.WithMaxRetries(3, retry.WithJitter(50*time.Millisecond, b))
}

func DefaultRedisRetry() retry.Backoff {
	b := retry.NewExponential(50 * time.Millisecond)
	return retry.WithMaxRetries(2, retry.WithJitter(25*time.Millisecond, b))
}

func ExternalAPIRetry() retry.Backoff {
	b := retry.NewExponential(500 * time.Millisecond)
	return retry.WithMaxRetries(2, retry.WithJitter(200*time.Millisecond, b))
}

func JitteredBackoff(base time.Duration, attempt int) time.Duration {
	backoff := base * time.Duration(1<<uint(attempt))
	jitter := time.Duration(rand.Int64N(int64(backoff) / 2))
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

func RetryableError(err error) error {
	if err == nil {
		return nil
	}
	return retry.RetryableError(err)
}

func MaybeRetryable(err error) error {
	if err == nil {
		return nil
	}
	if isRetryable(err) {
		return retry.RetryableError(err)
	}
	return err
}
