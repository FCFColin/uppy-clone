package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/uppy-clone/backend/internal/metrics"
)

// ObservePoolStats publishes pgx pool saturation metrics to Prometheus.
func (s *PostgresStore) ObservePoolStats() {
	p, ok := s.pool.(*pgxpool.Pool)
	if !ok {
		return
	}
	stat := p.Stat()
	metrics.DBPoolIdleConns.Set(float64(stat.IdleConns()))
	metrics.DBPoolInUseConns.Set(float64(stat.AcquiredConns()))
	s.recordAcquireDurationDelta(stat.AcquireDuration().Seconds(), stat.AcquireCount())
}

func (s *PostgresStore) recordAcquireDurationDelta(currentDuration float64, currentCount int64) {
	if prevDur, ok := s.lastAcquireDuration.Load().(float64); ok && prevDur > 0 {
		if prevCnt, ok := s.lastAcquireCount.Load().(int64); ok && currentCount > prevCnt {
			delta := currentDuration - prevDur
			countDelta := float64(currentCount - prevCnt)
			if delta > 0 && countDelta > 0 {
				metrics.DBPoolAcquireDuration.Observe(delta / countDelta)
			}
		}
	}
	s.lastAcquireDuration.Store(currentDuration)
	s.lastAcquireCount.Store(currentCount)
}