// Package resilience provides circuit breakers and retry helpers for downstream dependencies.
package resilience

import (
	"log/slog"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/metrics"
)

// Enterprise rationale: Circuit breaker prevents cascade failures (snowball effect).
// When a downstream dependency (DB, Redis, external API) becomes unhealthy,
// the breaker opens and returns errors immediately instead of waiting for timeouts.
// This protects upstream services and gives the downstream time to recover.
// Trade-off: During the open state, legitimate requests are rejected.
// Half-open state allows probing to detect recovery.

// 企业为何需要：熔断器状态变更必须可观测。否则运维无法知道下游依赖是否被熔断，也无法设置告警。

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
	// Set the new state gauge to its value, and reset the old state gauge to 0
	metrics.CircuitBreakerState.WithLabelValues(name, to.String()).Set(circuitBreakerStateValue(to))
	// Reset old state label to 0 so only the current state has a non-zero value
	if from != to {
		metrics.CircuitBreakerState.WithLabelValues(name, from.String()).Set(0)
	}
}

// Singleton circuit breakers (ADR-004): all stores/workers share one breaker per
// downstream dependency so that a tripped breaker is visible to health checks and
// prevents all components from hammering a failing dependency.
//
// Previously each repository and store called NewPostgresBreaker()/NewRedisBreaker()
// independently, creating 10+ fragmented breaker instances. When one tripped, the
// others kept sending requests, defeating the purpose of circuit breaking.

var (
	pgBreakerOnce      sync.Once
	pgBreakerSingleton *gobreaker.CircuitBreaker[any]

	redisBreakerOnce      sync.Once
	redisBreakerSingleton *gobreaker.CircuitBreaker[any]

	resendBreakerOnce      sync.Once
	resendBreakerSingleton *gobreaker.CircuitBreaker[any]
)

// NewPostgresBreaker returns the singleton circuit breaker for PostgreSQL access.
// All repositories share the same instance so that a tripped breaker affects all
// PG-dependent components and is visible to health checks (ADR-004, audit project-01-002).
func NewPostgresBreaker() *gobreaker.CircuitBreaker[any] {
	pgBreakerOnce.Do(func() {
		pgBreakerSingleton = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:        "postgres",
			MaxRequests: 3,                // Half-open: allow 3 probe requests
			Interval:    60 * time.Second, // Closed: count failures within this window
			Timeout:     30 * time.Second, // Open→Half-open: wait before probing
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5 // Open after 5 consecutive failures
			},
			OnStateChange: onStateChange,
		})
		// audit-020: Initialize the gauge to closed (0) at startup so Prometheus
		// reports the correct state before the first state change occurs.
		metrics.CircuitBreakerState.WithLabelValues("postgres", gobreaker.StateClosed.String()).Set(0)
	})
	return pgBreakerSingleton
}

// NewRedisBreaker returns the singleton circuit breaker for Redis access.
// All Redis stores share the same instance (ADR-004, audit project-01-002).
func NewRedisBreaker() *gobreaker.CircuitBreaker[any] {
	redisBreakerOnce.Do(func() {
		redisBreakerSingleton = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:        "redis",
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     15 * time.Second, // Redis typically recovers faster
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
			OnStateChange: onStateChange,
		})
		// audit-020: Initialize the gauge to closed (0) at startup.
		metrics.CircuitBreakerState.WithLabelValues("redis", gobreaker.StateClosed.String()).Set(0)
	})
	return redisBreakerSingleton
}

// NewResendBreaker returns the singleton circuit breaker for the Resend email API (ADR-004).
func NewResendBreaker() *gobreaker.CircuitBreaker[any] {
	resendBreakerOnce.Do(func() {
		resendBreakerSingleton = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:        "resend-api",
			MaxRequests: 1, // External API: be conservative
			Interval:    60 * time.Second,
			Timeout:     60 * time.Second, // External API: longer recovery wait
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 3 // External API: trip sooner
			},
			OnStateChange: onStateChange,
		})
		// audit-020: Initialize the gauge to closed (0) at startup.
		metrics.CircuitBreakerState.WithLabelValues("resend-api", gobreaker.StateClosed.String()).Set(0)
	})
	return resendBreakerSingleton
}

// ResetBreakersForTesting resets all singleton breakers. Test-only helper to
// ensure test isolation when tests trip breakers. Never call in production code.
func ResetBreakersForTesting() {
	pgBreakerOnce = sync.Once{}
	pgBreakerSingleton = nil
	redisBreakerOnce = sync.Once{}
	redisBreakerSingleton = nil
	resendBreakerOnce = sync.Once{}
	resendBreakerSingleton = nil
}
