package worker

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/util"
)

// claimLoopConfig parameterizes the shared XAUTOCLAIM background loop used by
// EmailWorker to reclaim zombie consumer messages.
type claimLoopConfig struct {
	stream     string // Redis stream name (e.g. "email:queue")
	group      string // consumer group name
	workerName string // log prefix (e.g. "email worker")
}

// Redis stream and consumer-group names used by the workers.
const (
	emailQueueStream  = "email:queue"
	emailWorkersGroup = "email-workers"
)

// runClaimPendingMessages periodically runs XAUTOCLAIM to reclaim messages
// stuck in the PEL (Pending Entries List) of consumers that crashed or became
// unresponsive, ensuring at-least-once delivery as required by ADR-007/009/010.
//
// Extracted from EmailWorker.claimPendingMessages to eliminate duplication (dupl).
func runClaimPendingMessages(
	ctx context.Context,
	rdb RedisStreamConsumer,
	consumerID string,
	cfg claimLoopConfig,
	process func(context.Context, redis.XMessage),
) {
	logger := util.LoggerFromContext(ctx)
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
