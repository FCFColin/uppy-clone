// Package outbox publishes transactional outbox events to Redis Streams.
package outbox

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/store/base"
	"github.com/uppy-clone/backend/internal/util"
)

// Publisher polls outbox_events and publishes to Redis Streams.
type Publisher struct {
	db        base.PGPool
	rdb       *redis.Client
	batchSize int
	interval  time.Duration
}

type outboxRow struct {
	id        int64
	aggType   string
	aggID     string
	payload   []byte
	createdAt int64
}

// NewPublisher creates a new Outbox Publisher.
func NewPublisher(db base.PGPool, streamer *redis.Client) *Publisher {
	batch := 100
	if v := os.Getenv("OUTBOX_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			batch = n
		}
	}
	interval := time.Second
	if v := os.Getenv("OUTBOX_POLL_INTERVAL_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			interval = time.Duration(ms) * time.Millisecond
		}
	}
	return &Publisher{db: db, rdb: streamer, batchSize: batch, interval: interval}
}

// Start begins polling outbox_events. Blocks until ctx is canceled.
func (p *Publisher) Start(ctx context.Context) {
	logger := util.LoggerFromContext(ctx).With("component", "outbox_publisher")
	ctx = util.WithLogger(ctx, logger)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.publishBatch(ctx)
		}
	}
}

func (p *Publisher) readPendingBatch(ctx context.Context, tx pgx.Tx) ([]outboxRow, int64) {
	logger := util.LoggerFromContext(ctx)
	rows, err := tx.Query(ctx,
		`SELECT id, aggregate_type, aggregate_id, payload, created_at
		 FROM outbox_events
		 WHERE processed_at IS NULL
		 ORDER BY id
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`, p.batchSize)
	if err != nil {
		logger.Error("outbox publisher: query", "error", err)
		return nil, 0
	}
	defer rows.Close()

	var batch []outboxRow
	var oldest int64
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.id, &r.aggType, &r.aggID, &r.payload, &r.createdAt); err != nil {
			logger.Error("outbox publisher: scan", "error", err)
			continue
		}
		if oldest == 0 || r.createdAt < oldest {
			oldest = r.createdAt
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("outbox publisher: rows iteration", "error", err)
		return nil, 0
	}
	return batch, oldest
}

func (p *Publisher) publishBatch(ctx context.Context) {
	logger := util.LoggerFromContext(ctx)
	tx, err := p.db.Begin(ctx)
	if err != nil {
		logger.Error("outbox publisher: begin tx", "error", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	batch, oldest := p.readPendingBatch(ctx, tx)
	if len(batch) == 0 {
		return
	}

	pipe := p.rdb.Pipeline()
	for _, item := range batch {
		stream := item.aggType + ".events"
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: stream,
			MaxLen: 100_000,
			Approx: true,
			Values: map[string]interface{}{
				"aggregate_id": item.aggID, //nolint:goconst // Redis stream field name (schema identifier)
				"event_id":     strconv.FormatInt(item.id, 10),
				"payload":      string(item.payload), //nolint:goconst // Redis stream field name (schema identifier)
			},
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		// audit-009: Increment metric so publish failures are visible in monitoring.
		metrics.OutboxPublishFailures.Inc()
		logger.Error("outbox publisher: pipeline XAdd", "error", err)
		return
	}

	now := time.Now().UnixMilli()
	if len(batch) > 0 {
		ids := make([]int64, len(batch))
		for i, item := range batch {
			ids[i] = item.id
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox_events SET processed_at = $1 WHERE id = ANY($2)`, now, ids); err != nil {
			logger.Error("outbox publisher: mark processed", "error", err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		logger.Error("outbox publisher: commit", "error", err)
		return
	}

	metrics.OutboxBatchSize.Observe(float64(len(batch)))
	if oldest > 0 {
		metrics.OutboxLagSeconds.Set(float64(now-oldest) / 1000)
	}
}
