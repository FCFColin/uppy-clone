package server

import (
	"context"
	"time"

	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/store"
)

// metricsCollectInterval controls ticker cadence; tests may shorten it for -short runs.
var metricsCollectInterval = appConfig.MetricsInterval

// startMetricsCollector starts all 3 Prometheus metrics goroutines.
func startMetricsCollector(ctx context.Context, hub *game.Hub, db *store.PostgresStore, cluster *store.RedisCluster) {
	runCollector(ctx, func() {
		metrics.ActiveRooms.Set(float64(hub.RoomCount()))
		metrics.ActivePlayers.Set(float64(hub.PlayerCount()))
		phaseCounts := hub.PhaseCounts()
		for _, phase := range []string{"waiting", "countdown", "playing", "ended"} {
			metrics.RoomsByPhase.WithLabelValues(phase).Set(float64(phaseCounts[phase]))
		}
		if streamLen, err := cluster.Stateful.Client().XLen(ctx, "game:results").Result(); err == nil {
			metrics.GameResultsStreamLen.Set(float64(streamLen))
		}
		if emailLen, err := cluster.Stateful.Client().XLen(ctx, "email:queue").Result(); err == nil {
			metrics.EmailQueueStreamLen.Set(float64(emailLen))
		}
	})

	runCollector(ctx, func() {
		db.ObservePoolStats()
	})

	runCollector(ctx, func() {
		stats := cluster.Stateful.PoolStats()
		metrics.RedisPoolIdleConns.Set(float64(stats.IdleConns))
		metrics.RedisPoolTotalConns.Set(float64(stats.TotalConns))
	})
}

func runCollector(ctx context.Context, fn func()) {
	go func() {
		ticker := time.NewTicker(metricsCollectInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fn()
			}
		}
	}()
}
