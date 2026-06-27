package server

import (
	"context"
	"time"

	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/store"
)

// startMetricsCollector starts all 3 Prometheus metrics goroutines.
func startMetricsCollector(ctx context.Context, hub *game.Hub, db *store.PostgresStore, redis *store.RedisStore) {
	// Periodically update business metrics for Prometheus
	go func() {
		ticker := time.NewTicker(appConfig.MetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.ActiveRooms.Set(float64(hub.RoomCount()))
				metrics.ActivePlayers.Set(float64(hub.PlayerCount()))
				phaseCounts := hub.PhaseCounts()
				for _, phase := range []string{"waiting", "countdown", "playing", "ended"} {
					metrics.RoomsByPhase.WithLabelValues(phase).Set(float64(phaseCounts[phase]))
				}

				// Monitor stream length for consumer lag
				if streamLen, err := redis.Client().XLen(ctx, "game:results").Result(); err == nil {
					metrics.GameResultsStreamLen.Set(float64(streamLen))
				}
				if emailLen, err := redis.Client().XLen(ctx, "email:queue").Result(); err == nil {
					metrics.EmailQueueStreamLen.Set(float64(emailLen))
				}
			}
		}
	}()

	// Periodically update DB pool metrics for Prometheus
	// Includes DBPoolAcquireDuration observation via delta sampling.
	go func() {
		ticker := time.NewTicker(appConfig.MetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				db.ObservePoolStats()
			}
		}
	}()

	// Periodically update Redis pool metrics for Prometheus
	go func() {
		ticker := time.NewTicker(appConfig.MetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := redis.PoolStats()
				metrics.RedisPoolIdleConns.Set(float64(stats.IdleConns))
				metrics.RedisPoolTotalConns.Set(float64(stats.TotalConns))
			}
		}
	}()
}
