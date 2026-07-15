package worker

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/slogctx"
)

// claimLoopConfig parameterizes the shared XAUTOCLAIM background loop used by
// both EmailWorker and GameResultWorker to reclaim zombie consumer messages.
type claimLoopConfig struct {
	stream     string // Redis stream name (e.g. "email:queue")
	group      string // consumer group name
	workerName string // log prefix (e.g. "email worker")
}

// runClaimPendingMessages periodically runs XAUTOCLAIM to reclaim messages
// stuck in the PEL (Pending Entries List) of consumers that crashed or became
// unresponsive, ensuring at-least-once delivery as required by ADR-007/009/010.
//
// Extracted from EmailWorker.claimPendingMessages and
// GameResultWorker.claimPendingMessages to eliminate duplication (dupl).
func runClaimPendingMessages(
	ctx context.Context,
	rdb RedisStreamConsumer,
	consumerID string,
	cfg claimLoopConfig,
	process func(context.Context, redis.XMessage),
) {
	logger := slogctx.LoggerFromContext(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	const (
		minIdleTime = 5 * time.Minute
		claimCount  = 100
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			autoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			msgs, _, err := rdb.XAutoClaim(autoCtx, &redis.XAutoClaimArgs{
				Stream:   cfg.stream,
				Group:    cfg.group,
				Consumer: consumerID,
				MinIdle:  minIdleTime,
				Count:    claimCount,
				Start:    "0",
			}).Result()
			cancel()
			if err != nil && err != redis.Nil {
				logger.Warn(cfg.workerName+" XAUTOCLAIM error", "error", err)
				continue
			}
			if len(msgs) > 0 {
				logger.Info(cfg.workerName+" XAUTOCLAIM reclaimed messages", "count", len(msgs))
				for _, msg := range msgs {
					process(ctx, msg)
				}
			}
		}
	}
}

// backoffSleep sleeps for the current backoff duration, then doubles it (capped
// at maxDur). Returns false if ctx is canceled during the sleep. Used by worker
// consume loops to avoid hammering Redis when it is degraded (v2-R-43).
func backoffSleep(ctx context.Context, backoff *time.Duration, maxDur time.Duration) bool {
	timer := time.NewTimer(*backoff)
	select {
	case <-ctx.Done():
		timer.Stop()
		return false
	case <-timer.C:
	}
	*backoff *= 2
	if *backoff > maxDur {
		*backoff = maxDur
	}
	return true
}
