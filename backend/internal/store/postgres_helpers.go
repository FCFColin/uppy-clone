package store

import (
	"context"
	"os"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/resilience"
)

func (s *PostgresStore) withRetryRead(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, err := s.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return resilience.MaybeRetryable(err)
	})
}

func (s *PostgresStore) withRetryWrite(ctx context.Context, fn func(context.Context) error) error {
	_, err := s.cb.Execute(func() (any, error) {
		return nil, fn(ctx)
	})
	return err
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultVal
}

func (s *PostgresStore) ObservePoolStats() {
	stat := s.pool.Stat()
	metrics.DBPoolIdleConns.Set(float64(stat.IdleConns()))
	metrics.DBPoolInUseConns.Set(float64(stat.AcquiredConns()))
	currentDuration := stat.AcquireDuration().Seconds()
	currentCount := stat.AcquireCount()
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
