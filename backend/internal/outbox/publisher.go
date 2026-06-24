package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Publisher polls outbox_events and publishes to Redis Streams.
// 企业为何需要：跨数据源（PG+Redis）的原子性无法用分布式事务保证。Outbox 模式将事件与业务数据
// 写入同一个 PG 事务，后台 publisher 轮询发布到 Redis Stream，保证 at-least-once 语义。
type Publisher struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

// NewPublisher creates a new Outbox Publisher.
func NewPublisher(db *pgxpool.Pool, rdb *redis.Client) *Publisher {
	return &Publisher{db: db, rdb: rdb}
}

// Start begins polling outbox_events. Blocks until ctx is canceled.
func (p *Publisher) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
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

func (p *Publisher) publishBatch(ctx context.Context) {
	rows, err := p.db.Query(ctx,
		`SELECT id, aggregate_type, aggregate_id, payload FROM outbox_events WHERE processed_at IS NULL ORDER BY id LIMIT 100`)
	if err != nil {
		slog.Error("outbox publisher: query", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var aggType, aggID string
		var payload []byte
		if err := rows.Scan(&id, &aggType, &aggID, &payload); err != nil {
			slog.Error("outbox publisher: scan", "error", err)
			continue
		}

		stream := aggType + ".events"
		err := p.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"aggregate_id": aggID,
				"payload":       string(payload),
			},
		}).Err()
		if err != nil {
			slog.Error("outbox publisher: XAdd", "error", err, "id", id)
			// Leave unprocessed — will be retried on next poll cycle.
			continue
		}

		// Mark as processed.
		// 企业为何需要：processed_at 标记确保已发布事件不会被重复发布。失败时事件保留，下次轮询重试。
		_, err = p.db.Exec(ctx, `UPDATE outbox_events SET processed_at = $1 WHERE id = $2`, time.Now().UnixMilli(), id)
		if err != nil {
			slog.Error("outbox publisher: mark processed", "error", err, "id", id)
		}
	}
}
