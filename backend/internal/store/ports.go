package store

import (
	"context"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop")

// PoolMetricsRecorder abstracts Prometheus metrics for the PG connection pool.
type PoolMetricsRecorder interface {
	IncAcquireCount()
	SetIdleConns(val float64)
	SetInUseConns(val float64)
	ObserveAcquireDuration(val float64)
}

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	Action    string
	ActorType string
	ActorID   string
	ActorIP   string
	Resource  string
	Before    interface{}
	After     interface{}
	RequestID string
	TraceID   string
}

const (
	ActorTypeSystem    = "system"
	ActorTypeUser      = "user"
	ActorTypeAdmin     = "admin"
	ActorTypeAnonymous = "anonymous"
)

type noopPoolMetrics struct{}

func (noopPoolMetrics) IncAcquireCount()               {}
func (noopPoolMetrics) SetIdleConns(float64)           {}
func (noopPoolMetrics) SetInUseConns(float64)          {}
func (noopPoolMetrics) ObserveAcquireDuration(float64) {}

// Deps holds all cross-cutting dependencies that store types need.
// The composition root (server_init.go) creates a Deps with real
// implementations; unit tests use DefaultDeps() which provides
// safe no-op defaults.
type Deps struct {
	PostgresBreakerFactory func() *gobreaker.CircuitBreaker[any]
	RedisBreakerFactory    func() *gobreaker.CircuitBreaker[any]
	DBRetryPolicy          retry.Backoff
	RedisRetryPolicy       retry.Backoff
	MaybeRetryableFn       func(err error) error
	Tracer                 trace.Tracer
	PoolMetrics            PoolMetricsRecorder
	AuditLogFn             func(context.Context, AuditEntry)
}

// DefaultDeps returns Deps populated with no-op/minimal implementations
// suitable for unit tests that don't need real resilience, tracing, or audit.
func DefaultDeps() Deps {
	return Deps{
		PostgresBreakerFactory: func() *gobreaker.CircuitBreaker[any] {
			return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{Name: "postgres"})
		},
		RedisBreakerFactory: func() *gobreaker.CircuitBreaker[any] {
			return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{Name: "redis"})
		},
		DBRetryPolicy:    retry.WithMaxRetries(3, retry.NewExponential(100*time.Millisecond)),
		RedisRetryPolicy: retry.WithMaxRetries(2, retry.NewExponential(50*time.Millisecond)),
		MaybeRetryableFn: func(err error) error { return err },
		Tracer:           noop.NewTracerProvider().Tracer("store"),
		PoolMetrics:      noopPoolMetrics{},
		AuditLogFn:       func(context.Context, AuditEntry) {},
	}
}

// depsOrZero returns the first deps if provided, otherwise DefaultDeps().
func depsOrZero(deps ...Deps) Deps {
	if len(deps) > 0 {
		return deps[0]
	}
	return DefaultDeps()
}
