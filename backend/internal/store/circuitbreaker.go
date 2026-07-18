package store

import (
	"log/slog"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/metrics"
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
			Name:        "postgres",
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
			OnStateChange: onStateChange,
		})
		metrics.CircuitBreakerState.WithLabelValues("postgres", gobreaker.StateClosed.String()).Set(0)
	})
	return pgBreakerSingleton
}

// NewRedisBreaker returns the singleton circuit breaker for Redis access.
func NewRedisBreaker() *gobreaker.CircuitBreaker[any] {
	redisBreakerOnce.Do(func() {
		redisBreakerSingleton = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:        "redis",
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     15 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
			OnStateChange: onStateChange,
		})
		metrics.CircuitBreakerState.WithLabelValues("redis", gobreaker.StateClosed.String()).Set(0)
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
