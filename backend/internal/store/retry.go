package store

import (
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"

	"github.com/uppy-clone/backend/internal/metrics"
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
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
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

// Circuit breaker name constants (used as breaker names and metric labels).
const (
	pgBreakerName    = "postgres"
	redisBreakerName = "redis"
)

// circuitBreakerStateValue maps gobreaker states to Prometheus gauge values.
func circuitBreakerStateValue(state gobreaker.State) float64 {
	switch state {
	case gobreaker.StateClosed:
		return 0
	case gobreaker.StateHalfOpen:
		return 0.5
	case gobreaker.StateOpen:
		return 1
	default:
		return -1
	}
}

// onStateChange logs the state change and updates the CircuitBreakerState gauge.
func onStateChange(name string, from gobreaker.State, to gobreaker.State) {
	slog.Warn("circuit breaker state changed",
		"breaker", name,
		"from", from.String(),
		"to", to.String(),
	)
	metrics.CircuitBreakerState.WithLabelValues(name, to.String()).Set(circuitBreakerStateValue(to))
	if from != to {
		metrics.CircuitBreakerState.WithLabelValues(name, from.String()).Set(0)
	}
}

var (
	pgBreakerOnce      sync.Once
	pgBreakerSingleton *gobreaker.CircuitBreaker[any]

	redisBreakerOnce      sync.Once
	redisBreakerSingleton *gobreaker.CircuitBreaker[any]

	resendBreakerOnce      sync.Once
	resendBreakerSingleton *gobreaker.CircuitBreaker[any]
)

// NewPostgresBreaker returns the singleton circuit breaker for PostgreSQL access.
func NewPostgresBreaker() *gobreaker.CircuitBreaker[any] {
	pgBreakerOnce.Do(func() {
		pgBreakerSingleton = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:        pgBreakerName,
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
			OnStateChange: onStateChange,
		})
		metrics.CircuitBreakerState.WithLabelValues(pgBreakerName, gobreaker.StateClosed.String()).Set(0)
	})
	return pgBreakerSingleton
}

// NewRedisBreaker returns the singleton circuit breaker for Redis access.
func NewRedisBreaker() *gobreaker.CircuitBreaker[any] {
	redisBreakerOnce.Do(func() {
		redisBreakerSingleton = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:        redisBreakerName,
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     15 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
			OnStateChange: onStateChange,
		})
		metrics.CircuitBreakerState.WithLabelValues(redisBreakerName, gobreaker.StateClosed.String()).Set(0)
	})
	return redisBreakerSingleton
}

// NewResendBreaker returns the singleton circuit breaker for the Resend email API.
func NewResendBreaker() *gobreaker.CircuitBreaker[any] {
	resendBreakerOnce.Do(func() {
		resendBreakerSingleton = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:        "resend-api",
			MaxRequests: 1,
			Interval:    60 * time.Second,
			Timeout:     60 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 3
			},
			OnStateChange: onStateChange,
		})
		metrics.CircuitBreakerState.WithLabelValues("resend-api", gobreaker.StateClosed.String()).Set(0)
	})
	return resendBreakerSingleton
}

// ResetBreakersForTesting resets all singleton breakers. Test-only helper.
func ResetBreakersForTesting() {
	pgBreakerOnce = sync.Once{}
	pgBreakerSingleton = nil
	redisBreakerOnce = sync.Once{}
	redisBreakerSingleton = nil
	resendBreakerOnce = sync.Once{}
	resendBreakerSingleton = nil
}
