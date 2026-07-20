package bootstrap

import (
	"context"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
)

// PoolMetricsAdapter adapts the metrics package's Prometheus collectors to
// the store.PoolMetricsRecorder interface (RO-052). Used by both server and
// worker store-deps builders.
//
// Previously duplicated as server.poolMetricsAdapter and
// worker.workerPoolMetricsAdapter (4 methods, only the type name differed).
type PoolMetricsAdapter struct{}

// IncAcquireCount increments the DB pool acquire counter.
func (PoolMetricsAdapter) IncAcquireCount() { metrics.DBPoolAcquireCount.Inc() }

// SetIdleConns sets the idle connection gauge.
func (PoolMetricsAdapter) SetIdleConns(v float64) { metrics.DBPoolIdleConns.Set(v) }

// SetInUseConns sets the in-use connection gauge.
func (PoolMetricsAdapter) SetInUseConns(v float64) { metrics.DBPoolInUseConns.Set(v) }

// ObserveAcquireDuration records a DB pool acquire latency sample.
func (PoolMetricsAdapter) ObserveAcquireDuration(v float64) {
	metrics.DBPoolAcquireDuration.Observe(v)
}

// NewStoreDeps builds production store.Deps with real resilience, tracing,
// metrics, and audit logging. Passed to store constructors via variadic
// parameter.
//
// Previously duplicated as server.newStoreDeps and worker.newWorkerStoreDeps
// (30+ lines of byte-level identical code).
func NewStoreDeps() store.Deps {
	return store.Deps{
		PostgresBreakerFactory: store.NewPostgresBreaker,
		RedisBreakerFactory:    store.NewRedisBreaker,
		DBRetryPolicy:          store.DefaultDBRetry(),
		RedisRetryPolicy:       store.DefaultRedisRetry(),
		MaybeRetryableFn:       store.MaybeRetryable,
		Tracer:                 telemetry.Tracer(),
		PoolMetrics:            PoolMetricsAdapter{},
		AuditLogFn: func(ctx context.Context, entry store.AuditEntry) {
			audit.Log(ctx, entry)
		},
	}
}
