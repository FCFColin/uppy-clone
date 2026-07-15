package worker

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/slogctx"
)

// RedisStreamConsumer is the narrow subset of *redis.Client methods used by
// EmailWorker and GameResultWorker for Redis Stream consumption (RO-051).
// Abstracting behind an interface prevents raw *redis.Client penetration —
// workers receive a contract, not the full client. *redis.Client satisfies
// this interface automatically, so no adapter is needed.
type RedisStreamConsumer interface {
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd
	XReadGroup(ctx context.Context, a *redis.XReadGroupArgs) *redis.XStreamSliceCmd
	XAutoClaim(ctx context.Context, a *redis.XAutoClaimArgs) *redis.XAutoClaimCmd
	XAck(ctx context.Context, stream, group string, ids ...string) *redis.IntCmd
	XAdd(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd
}

// handleRetry re-enqueues msg to sourceStream with an incremented retry_count using
// exponential backoff (100ms * 2^retryCount). After maxRetries attempts, the message
// is moved to deadLetterStream and acked from the source group.
//
// Returns true when the message was moved to the dead-letter stream (v2-R-43 metric hook).
func handleRetry(
	ctx context.Context,
	rdb RedisStreamConsumer,
	msg redis.XMessage,
	sourceStream, sourceGroup, deadLetterStream string,
	maxRetries int,
	logAttrs ...any,
) bool {
	retryCount := parseRetryCount(msg)
	if retryCount >= maxRetries {
		return moveToDeadLetter(ctx, rdb, msg, sourceStream, sourceGroup, deadLetterStream, retryCount, logAttrs)
	}
	return reenqueueWithBackoff(ctx, rdb, msg, sourceStream, sourceGroup, retryCount, logAttrs)
}

// parseRetryCount extracts the retry_count field from the message metadata,
// returning 0 if absent or unparseable.
func parseRetryCount(msg redis.XMessage) int {
	if rcStr, ok := msg.Values["retry_count"].(string); ok {
		if n, err := strconv.Atoi(rcStr); err == nil {
			return n
		}
	}
	return 0
}

// moveToDeadLetter transfers msg to the dead-letter stream and acks it from the
// source group. Returns true when the message was successfully dead-lettered.
func moveToDeadLetter(
	ctx context.Context,
	rdb RedisStreamConsumer,
	msg redis.XMessage,
	sourceStream, sourceGroup, deadLetterStream string,
	retryCount int,
	logAttrs []any,
) bool {
	logger := slogctx.LoggerFromContext(ctx)
	err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: deadLetterStream,
		MaxLen: 10_000,
		Approx: true,
		Values: msg.Values,
	}).Err()
	if err != nil {
		logger.Error("failed to move to dead-letter, message remains in stream", "error", err, "id", msg.ID)
		metrics.WorkerAckErrors.WithLabelValues("email").Inc()
		return false
	}
	if ackErr := rdb.XAck(ctx, sourceStream, sourceGroup, msg.ID).Err(); ackErr != nil {
		logger.Error("failed to ack dead-lettered message", "error", ackErr, "id", msg.ID)
		metrics.WorkerAckErrors.WithLabelValues("email").Inc()
	}
	attrs := append([]any{"id", msg.ID, "retries", retryCount}, logAttrs...)
	logger.Error("moved to dead-letter after max retries", attrs...)
	return true
}

// reenqueueWithBackoff sleeps for an exponential backoff (100ms * 2^retryCount,
// shift capped at 30 to prevent Duration overflow — audit-024), then re-adds
// msg to the source stream with an incremented retry_count and acks the
// original. Returns false (the message was not dead-lettered).
func reenqueueWithBackoff(
	ctx context.Context,
	rdb RedisStreamConsumer,
	msg redis.XMessage,
	sourceStream, sourceGroup string,
	retryCount int,
	_ []any,
) bool {
	logger := slogctx.LoggerFromContext(ctx)
	shift := retryCount
	if shift > 30 {
		shift = 30
	}
	delay := time.Duration(1<<shift) * 100 * time.Millisecond
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
		MaxLen: 10_000, // audit-015: Cap stream length to prevent unbounded growth
		Approx: true,
		Values: msg.Values,
	}).Err()
	if err != nil {
		logger.Error("failed to re-enqueue", "error", err, "id", msg.ID, "retries", retryCount)
		return false
	}
	if ackErr := rdb.XAck(ctx, sourceStream, sourceGroup, msg.ID).Err(); ackErr != nil {
		logger.Error("failed to ack re-enqueued message", "error", ackErr, "id", msg.ID)
		metrics.WorkerAckErrors.WithLabelValues("email").Inc()
	}
	return false
}
