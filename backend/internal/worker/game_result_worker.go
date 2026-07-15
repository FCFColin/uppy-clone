package worker

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/slogctx"
)

// gameResultWorkerName is the metrics label value for the game result worker.
const gameResultWorkerName = "game_result"

// gameResultDB begins transactions for persisting game results.
type gameResultDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// GameResultWorker consumes game.events Redis Stream and batch-inserts into PostgreSQL.
// 企业为何需要：游戏结束热路径不应被 PG 写入延迟阻塞，批量写入提升吞吐 10-50x。
type GameResultWorker struct {
	rdb        RedisStreamConsumer
	db         gameResultDB
	maxRetries int
	consumerID string
}

// NewGameResultWorker creates a new GameResultWorker.
func NewGameResultWorker(rdb RedisStreamConsumer, db *pgxpool.Pool) *GameResultWorker {
	return &GameResultWorker{
		rdb:        rdb,
		db:         db,
		maxRetries: 5,
		consumerID: resolveGameResultConsumerID(),
	}
}

// resolveGameResultConsumerID returns a consumer identifier for the game result worker.
// Precedence (v2-R-42): GAME_RESULT_WORKER_CONSUMER_ID env > HOSTNAME env (pod name in K8s)
// > os.Hostname() > fallback "1".
func resolveGameResultConsumerID() string {
	if v := os.Getenv("GAME_RESULT_WORKER_CONSUMER_ID"); v != "" {
		return v
	}
	if h := os.Getenv("HOSTNAME"); h != "" {
		return "result-worker-" + h
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return "result-worker-" + h
	}
	return "result-worker-1"
}

// GameResultPayload is the message format for game results.
type GameResultPayload struct {
	GameID     string             `json:"game_id"` // session_id
	RoomCode   string             `json:"room_code"`
	FinalScore int                `json:"final_score"`
	Results    []PlayerGameResult `json:"results"`
	EndedAt    int64              `json:"ended_at"`
}

// PlayerGameResult represents a single player's result in a game session.
type PlayerGameResult struct {
	UserID            string `json:"user_id"`
	ScoreContribution int    `json:"score_contribution"`
	TapsCount         int    `json:"taps_count"`
}

// outboxEventEnvelope wraps the game result payload in an outbox event envelope.
// The outbox Publisher publishes events as {"event":"game.ended","data":{...}}.
// RO-043: The Worker now consumes from "game.events" (published by the outbox
// Publisher) instead of the old "game:results" stream (written directly by game code).
type outboxEventEnvelope struct {
	Event string            `json:"event"`
	Data  GameResultPayload `json:"data"`
}

// Start begins consuming the game results queue. Blocks until ctx is canceled.
//
// Backoff (v2-R-43): on XReadGroup errors, sleep with exponential backoff
// (capped at maxReadBackoff) to avoid hammering Redis when it is degraded.
func (w *GameResultWorker) Start(ctx context.Context) {
	logger := slogctx.LoggerFromContext(ctx).With("worker", gameResultWorkerName, "consumer", w.consumerID)
	ctx = slogctx.WithLogger(ctx, logger)

	if err := w.rdb.XGroupCreateMkStream(ctx, "game.events", "result-workers", "$").Err(); err != nil {
		// audit-023: Upgrade from Debug to Warn — see email_worker.go for rationale.
		logger.Warn("game result worker: XGroupCreate failed (may already exist)", "error", err)
	}

	// audit-003: Start XAUTOCLAIM background goroutine to reclaim messages
	// stuck in the PEL of consumers that crashed or became unresponsive.
	go w.claimPendingMessages(ctx)

	w.consumeLoop(ctx)
}

// consumeLoop reads messages from the game.events stream in batches, flushing
// either when a batch reaches 100 messages or every 1 second (whichever comes
// first). On XReadGroup errors it backs off exponentially (v2-R-43). Blocks
// until ctx is canceled; on exit it flushes any remaining buffered messages.
func (w *GameResultWorker) consumeLoop(ctx context.Context) {
	batch := make([]redis.XMessage, 0, 100)
	flushTimer := time.NewTicker(1 * time.Second)
	defer flushTimer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.processBatch(ctx, batch)
		batch = batch[:0]
	}

	backoff := 100 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-flushTimer.C:
			flush()
		default:
		}

		streams, done := w.readBatch(ctx, &backoff)
		if done {
			return
		}
		for _, stream := range streams {
			batch = append(batch, stream.Messages...)
			if len(batch) >= 100 {
				flush()
			}
		}
	}
}

// readBatch performs one XReadGroup call and handles exponential backoff on
// errors. Returns the streams read (possibly nil on error or redis.Nil timeout)
// and done=true if ctx was canceled during backoff (caller should return).
// On success or redis.Nil the backoff is reset to its initial value.
func (w *GameResultWorker) readBatch(ctx context.Context, backoff *time.Duration) (streams []redis.XStream, done bool) {
	logger := slogctx.LoggerFromContext(ctx)
	const (
		initialBackoff = 100 * time.Millisecond
		maxBackoff     = 10 * time.Second
	)
	streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    "result-workers",
		Consumer: w.consumerID,
		Streams:  []string{"game.events", ">"},
		Count:    100,
		Block:    100 * time.Millisecond,
	}).Result()
	if err != nil && err != redis.Nil {
		logger.Error("game result worker XReadGroup", "error", err)
		metrics.WorkerReadErrors.WithLabelValues(gameResultWorkerName).Inc()
		if !backoffSleep(ctx, backoff, maxBackoff) {
			return nil, true
		}
		return nil, false
	}
	*backoff = initialBackoff
	return streams, false
}

// claimPendingMessages periodically runs XAUTOCLAIM to reclaim messages
// stuck in the PEL of consumers that crashed or became unresponsive.
// audit-003: Without this, a crash mid-processing leaves messages as zombies
// in the PEL forever, violating at-least-once delivery (ADR-007/009/010).
func (w *GameResultWorker) claimPendingMessages(ctx context.Context) {
	runClaimPendingMessages(ctx, w.rdb, w.consumerID, claimLoopConfig{
		stream:     "game.events",
		group:      "result-workers",
		workerName: "game result worker",
	}, w.processMessage)
}

// processBatch handles a batch of game result messages.
// audit-012: Each message is processed in its own transaction rather than
// batching all messages into a single transaction. This is a deliberate
// trade-off: per-message transactions ensure that one malformed message
// does not abort the entire batch (PostgreSQL aborts the whole transaction
// on any error). Batching would require savepoints or per-message error
// isolation, adding complexity for marginal throughput gain since each
// message already contains a full game session with multiple player results
// that ARE batched in a single transaction.
func (w *GameResultWorker) processBatch(ctx context.Context, messages []redis.XMessage) {
	for _, msg := range messages {
		w.processMessage(ctx, msg)
	}
}

func (w *GameResultWorker) ackMessage(ctx context.Context, id string) {
	if w.rdb != nil {
		// audit-018: Handle XAck errors — previously the return value was
		// completely ignored, leaving messages in PEL without any metric/log.
		if err := w.rdb.XAck(ctx, "game.events", "result-workers", id).Err(); err != nil {
			slogctx.LoggerFromContext(ctx).Error("game result worker: XAck error", "error", err, "id", id)
			metrics.WorkerAckErrors.WithLabelValues(gameResultWorkerName).Inc()
		}
	}
}

// processMessage handles a single game result message in its own transaction.
// 每条消息独立事务，避免一条失败导致整批事务中止（PostgreSQL 行为）。
// 只有写入成功后才 XAck，保证 at-least-once 语义。
func (w *GameResultWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	start := time.Now()

	payload, ok := w.parseGameResultPayload(ctx, msg, start)
	if !ok {
		return
	}

	if err := w.persistGameResult(ctx, payload, msg, start); err != nil {
		return
	}

	w.recordSuccess(ctx, msg, start)
}

// parseGameResultPayload extracts and validates the game result payload from the
// stream message. On invalid payload it acks the message (so it is not retried)
// and records the invalid_payload metric. Returns ok=false on failure.
func (w *GameResultWorker) parseGameResultPayload(ctx context.Context, msg redis.XMessage, start time.Time) (GameResultPayload, bool) {
	logger := slogctx.LoggerFromContext(ctx)
	payloadStr, ok := msg.Values["payload"].(string)
	if !ok {
		logger.Error("game result worker: invalid payload", "id", msg.ID)
		w.recordInvalidPayload(ctx, msg, start)
		return GameResultPayload{}, false
	}
	var envelope outboxEventEnvelope
	if err := json.Unmarshal([]byte(payloadStr), &envelope); err != nil {
		logger.Error("game result worker: unmarshal envelope", "error", err, "id", msg.ID)
		w.recordInvalidPayload(ctx, msg, start)
		return GameResultPayload{}, false
	}
	payload := envelope.Data
	if _, err := uuid.Parse(payload.GameID); err != nil {
		logger.Error("game result worker: invalid game_id", "error", err, "id", msg.ID)
		w.recordInvalidPayload(ctx, msg, start)
		return GameResultPayload{}, false
	}
	return payload, true
}

// recordInvalidPayload acks an invalid payload message and records the
// invalid_payload metric + processing duration.
func (w *GameResultWorker) recordInvalidPayload(ctx context.Context, msg redis.XMessage, start time.Time) {
	w.ackMessage(ctx, msg.ID)
	metrics.WorkerMessagesProcessed.WithLabelValues(gameResultWorkerName, "invalid_payload").Inc()
	metrics.WorkerProcessingDuration.WithLabelValues(gameResultWorkerName).Observe(time.Since(start).Seconds())
}

// recordTransientFailure invokes retry/dead-letter handling and records the
// failure metric + processing duration.
func (w *GameResultWorker) recordTransientFailure(ctx context.Context, msg redis.XMessage, start time.Time) {
	w.handleTransientFailure(ctx, msg)
	metrics.WorkerMessagesProcessed.WithLabelValues(gameResultWorkerName, "failure").Inc()
	metrics.WorkerProcessingDuration.WithLabelValues(gameResultWorkerName).Observe(time.Since(start).Seconds())
}

// recordSuccess acks the message and records the success metric + duration.
func (w *GameResultWorker) recordSuccess(ctx context.Context, msg redis.XMessage, start time.Time) {
	w.ackMessage(ctx, msg.ID)
	metrics.WorkerMessagesProcessed.WithLabelValues(gameResultWorkerName, "success").Inc()
	metrics.WorkerProcessingDuration.WithLabelValues(gameResultWorkerName).Observe(time.Since(start).Seconds())
}

// persistGameResult saves the game result in its own transaction: it upserts
// the game session and inserts each player result. On any error it records a
// transient failure (retry/dead-letter) and returns a non-nil error so the
// caller skips the success path. The deferred Rollback is a no-op after a
// successful Commit.
func (w *GameResultWorker) persistGameResult(ctx context.Context, payload GameResultPayload, msg redis.XMessage, start time.Time) error {
	logger := slogctx.LoggerFromContext(ctx)
	tx, err := w.db.Begin(ctx)
	if err != nil {
		logger.Error("game result worker: begin tx", "error", err)
		w.recordTransientFailure(ctx, msg, start)
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// UPSERT：若会话行不存在（例如创建时因约束失败未写入），直接以 ended 状态插入；
	// 若已存在则更新为 ended。避免 FK 违约导致 game_results 插入失败。
	// audit-016: Only set status='ended' when current status is 'active'.
	// Previously the UPSERT unconditionally overwrote status, which could
	// clobber a 'cancelled' or 'abandoned' status set by another process.
	if _, err := tx.Exec(ctx,
		`INSERT INTO game_sessions (id, lobby_code, status, ended_at, final_score)
		 VALUES ($1, $2, 'ended', $3, $4)
		 ON CONFLICT (id) DO UPDATE SET
		     status = CASE WHEN game_sessions.status = 'active' THEN 'ended' ELSE game_sessions.status END,
		     ended_at = EXCLUDED.ended_at,
		     final_score = EXCLUDED.final_score`,
		payload.GameID, payload.RoomCode, payload.EndedAt, payload.FinalScore); err != nil {
		logger.Error("game result worker: upsert session", "error", err, "game_id", payload.GameID)
		w.recordTransientFailure(ctx, msg, start)
		return err
	}

	for _, r := range payload.Results {
		resultID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(payload.GameID+r.UserID)).String()
		if _, err := tx.Exec(ctx,
			`INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO NOTHING`,
			resultID, payload.GameID, r.UserID, r.ScoreContribution, r.TapsCount, payload.EndedAt); err != nil {
			logger.Error("game result worker: insert result", "error", err, "game_id", payload.GameID)
			w.recordTransientFailure(ctx, msg, start)
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logger.Error("game result worker: commit", "error", err, "game_id", payload.GameID)
		w.recordTransientFailure(ctx, msg, start)
		return err
	}
	return nil
}

func (w *GameResultWorker) handleTransientFailure(ctx context.Context, msg redis.XMessage) {
	deadLettered := handleRetry(ctx, w.rdb, msg, "game.events", "result-workers", "game-result:dead-letter", w.maxRetries, "worker", "game-result")
	if deadLettered {
		metrics.WorkerMessagesProcessed.WithLabelValues(gameResultWorkerName, "deadletter").Inc()
	}
}
