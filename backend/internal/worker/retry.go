package worker

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func handleRetry(
	ctx context.Context,
	rdb *redis.Client,
	msg redis.XMessage,
	sourceStream, sourceGroup, deadLetterStream string,
	maxRetries int,
	logAttrs ...any,
) {
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
			slog.Error("failed to move to dead-letter", "error", err, "id", msg.ID)
			return
		}
		rdb.XAck(ctx, sourceStream, sourceGroup, msg.ID)
		attrs := append([]any{"id", msg.ID, "retries", retryCount}, logAttrs...)
		slog.Error("moved to dead-letter after max retries", attrs...)
		return
	}

	delay := time.Duration(1<<retryCount) * 100 * time.Millisecond
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return
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
		slog.Error("failed to re-enqueue", "error", err, "id", msg.ID, "retries", retryCount)
		return
	}
	rdb.XAck(ctx, sourceStream, sourceGroup, msg.ID)
}
