package worker

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/slogctx"
)

// handleRetry re-enqueues msg to sourceStream with an incremented retry_count using
// exponential backoff (100ms * 2^retryCount). After maxRetries attempts, the message
// is moved to deadLetterStream and acked from the source group.
//
// Returns true when the message was moved to the dead-letter stream (v2-R-43 metric hook).
func handleRetry(
	ctx context.Context,
	rdb *redis.Client,
	msg redis.XMessage,
	sourceStream, sourceGroup, deadLetterStream string,
	maxRetries int,
	logAttrs ...any,
) bool {
	logger := slogctx.LoggerFromContext(ctx)
	retryCount := 0
	if rcStr, ok := msg.Values["retry_count"].(string); ok {
		if n, err := strconv.Atoi(rcStr); err == nil {
			retryCount = n
		}
	}

	if retryCount >= maxRetries {
		err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: deadLetterStream,
			MaxLen: 10_000,
			Approx: true,
			Values: msg.Values,
		}).Err()
		if err != nil {
			logger.Error("failed to move to dead-letter", "error", err, "id", msg.ID)
			return true
		}
		rdb.XAck(ctx, sourceStream, sourceGroup, msg.ID)
		attrs := append([]any{"id", msg.ID, "retries", retryCount}, logAttrs...)
		logger.Error("moved to dead-letter after max retries", attrs...)
		return true
	}

	// Exponential backoff: 100ms, 200ms, 400ms, 800ms, ... capped by caller's maxRetries.
	delay := time.Duration(1<<retryCount) * 100 * time.Millisecond
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return false
	case <-timer.C:
	}

	msg.Values["retry_count"] = strconv.Itoa(retryCount + 1)
	err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: sourceStream,
		MaxLen: 100_000,
		Approx: true,
		Values: msg.Values,
	}).Err()
	if err != nil {
		logger.Error("failed to re-enqueue", "error", err, "id", msg.ID, "retries", retryCount)
		return false
	}
	rdb.XAck(ctx, sourceStream, sourceGroup, msg.ID)
	return false
}
