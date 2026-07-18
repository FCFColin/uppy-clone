package bootstrap

import "github.com/uppy-clone/backend/internal/metrics"

// PoolMetricsAdapter adapts the metrics package's Prometheus collectors to
// the base.PoolMetricsRecorder interface (RO-052). Used by both server and
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
