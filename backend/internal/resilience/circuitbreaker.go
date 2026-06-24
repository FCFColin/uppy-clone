package resilience

import (
	"log/slog"
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

func NewPostgresBreaker() *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        "postgres",
		MaxRequests: 3, // Half-open: allow 3 probe requests
		Interval:    60 * time.Second, // Closed: count failures within this window
		Timeout:     30 * time.Second, // Open→Half-open: wait before probing
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 5 // Open after 5 consecutive failures
		},
		OnStateChange: onStateChange,
	})
}

func NewRedisBreaker() *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        "redis",
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     15 * time.Second, // Redis typically recovers faster
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 5
		},
		OnStateChange: onStateChange,
	})
}

func NewResendBreaker() *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        "resend-api",
		MaxRequests: 1, // External API: be conservative
		Interval:    60 * time.Second,
		Timeout:     60 * time.Second, // External API: longer recovery wait
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 3 // External API: trip sooner
		},
		OnStateChange: onStateChange,
	})
}
