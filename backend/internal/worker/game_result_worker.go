package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// gameResultDB begins transactions for persisting game results.
type gameResultDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// GameResultWorker consumes game:results Redis Stream and batch-inserts into PostgreSQL.
// 企业为何需要：游戏结束热路径不应被 PG 写入延迟阻塞，批量写入提升吞吐 10-50x。
type GameResultWorker struct {
	rdb *redis.Client
	db  gameResultDB
}

// NewGameResultWorker creates a new GameResultWorker.
func NewGameResultWorker(rdb *redis.Client, db *pgxpool.Pool) *GameResultWorker {
	return &GameResultWorker{rdb: rdb, db: db}
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

// Start begins consuming the game results queue. Blocks until ctx is canceled.
func (w *GameResultWorker) Start(ctx context.Context) {
	if err := w.rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "$").Err(); err != nil {
		slog.Debug("game result worker: XGroupCreate (may already exist)", "error", err)
	}

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

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-flushTimer.C:
			flush()
		default:
		}

		consumer := "result-worker-" + os.Getenv("HOSTNAME")
		if consumer == "result-worker-" {
			consumer = "result-worker-1"
		}

		streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    "result-workers",
			Consumer: consumer,
			Streams:  []string{"game:results", ">"},
			Count:    100,
			Block:    100 * time.Millisecond,
		}).Result()
		if err != nil && err != redis.Nil {
			slog.Error("game result worker XReadGroup", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			batch = append(batch, stream.Messages...)
			if len(batch) >= 100 {
				flush()
			}
		}
	}
}

func (w *GameResultWorker) processBatch(ctx context.Context, messages []redis.XMessage) {
	for _, msg := range messages {
		w.processMessage(ctx, msg)
	}
}

func (w *GameResultWorker) ackMessage(ctx context.Context, id string) {
	if w.rdb != nil {
		w.rdb.XAck(ctx, "game:results", "result-workers", id)
	}
}

// processMessage handles a single game result message in its own transaction.
// 每条消息独立事务，避免一条失败导致整批事务中止（PostgreSQL 行为）。
// 只有写入成功后才 XAck，保证 at-least-once 语义。
func (w *GameResultWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	payloadStr, ok := msg.Values["payload"].(string)
	if !ok {
		slog.Error("game result worker: invalid payload", "id", msg.ID)
		w.ackMessage(ctx, msg.ID)
		return
	}
	var payload GameResultPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		slog.Error("game result worker: unmarshal", "error", err, "id", msg.ID)
		w.ackMessage(ctx, msg.ID)
		return
	}
	if _, err := uuid.Parse(payload.GameID); err != nil {
		slog.Error("game result worker: invalid game_id", "error", err, "id", msg.ID)
		w.ackMessage(ctx, msg.ID)
		return
	}

	tx, err := w.db.Begin(ctx)
	if err != nil {
		slog.Error("game result worker: begin tx", "error", err)
		return // 不 ACK，等待重试
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// UPSERT：若会话行不存在（例如创建时因约束失败未写入），直接以 ended 状态插入；
	// 若已存在则更新为 ended。避免 FK 违约导致 game_results 插入失败。
	if _, err := tx.Exec(ctx,
		`INSERT INTO game_sessions (id, lobby_code, status, ended_at, final_score)
		 VALUES ($1, $2, 'ended', $3, $4)
		 ON CONFLICT (id) DO UPDATE SET status = 'ended', ended_at = EXCLUDED.ended_at, final_score = EXCLUDED.final_score`,
		payload.GameID, payload.RoomCode, payload.EndedAt, payload.FinalScore); err != nil {
		slog.Error("game result worker: upsert session", "error", err, "game_id", payload.GameID)
		return // 不 ACK，等待重试
	}

	for _, r := range payload.Results {
		resultID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(payload.GameID+r.UserID)).String()
		if _, err := tx.Exec(ctx,
			`INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO NOTHING`,
			resultID, payload.GameID, r.UserID, r.ScoreContribution, r.TapsCount, payload.EndedAt); err != nil {
			slog.Error("game result worker: insert result", "error", err, "game_id", payload.GameID)
			return // 不 ACK，等待重试
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("game result worker: commit", "error", err, "game_id", payload.GameID)
		return // 不 ACK，等待重试
	}

	w.ackMessage(ctx, msg.ID)
}
