package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/uppy-clone/backend/internal/audit"
)

// AuditEntry is an alias for audit.AuditEntry so callers constructing
// store.Deps (e.g. server_init, bootstrap) can reference the type without
// importing the audit package directly.
type AuditEntry = audit.AuditEntry

// ─── Pool & Metrics ──────────────────────────────────────────────────

// PGPool abstracts pgxpool for store operations (enables pgxmock in unit tests).
type PGPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Close()
	Ping(ctx context.Context) error
}

// PoolMetricsRecorder abstracts Prometheus metrics for the PG connection pool.
type PoolMetricsRecorder interface {
	IncAcquireCount()
	SetIdleConns(val float64)
	SetInUseConns(val float64)
	ObserveAcquireDuration(val float64)
}

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
			return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{Name: pgBreakerName})
		},
		RedisBreakerFactory: func() *gobreaker.CircuitBreaker[any] {
			return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{Name: redisBreakerName})
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
