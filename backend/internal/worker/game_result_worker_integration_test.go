//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/testutil"
)

func setupPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testutil.SetupPostgres(t, testutil.WithPool(), testutil.WithMigrations()).Pool
}

func setupRedisForWorker(t *testing.T) *redis.Client {
	t.Helper()
	rdb, _ := testutil.SetupRedisClient(t)
	return rdb
}

// setupGameTestData creates a user and game session in the DB for testing.
// Returns (userID, sessionID).
func setupGameTestData(t *testing.T, pool *pgxpool.Pool) (userID, sessionID string) {
	t.Helper()
	ctx := context.Background()

	userID = uuid.NewString()
	sessionID = uuid.NewString()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, nickname, palette, created_at) VALUES ($1, $2, $3, $4, $5)`,
		userID, fmt.Sprintf("test-%s@example.com", userID[:8]), "TestUser", 0, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO game_sessions (id, lobby_code, created_by, status, started_at) VALUES ($1, $2, $3, $4, $5)`,
		sessionID, "ROOM1", userID, "active", time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("create game session: %v", err)
	}

	return userID, sessionID
}

// makeGameResultPayload creates a JSON-encoded outbox envelope wrapping a GameResultPayload.
// RO-043: The Worker now consumes from game.events (published by the outbox Publisher),
// so test payloads must be wrapped in {"event":"game.ended","data":{...}} format.
func makeGameResultPayload(gameID string, finalScore int, results []PlayerGameResult, endedAt int64) string {
	p := GameResultPayload{
		GameID:     gameID,
		RoomCode:   "ROOM1",
		FinalScore: finalScore,
		Results:    results,
		EndedAt:    endedAt,
	}
	env := outboxEventEnvelope{Event: "game.ended", Data: p}
	b, _ := json.Marshal(env)
	return string(b)
}

// ensureResultWorkerGroup creates the game:results consumer group when missing.
func ensureResultWorkerGroup(t *testing.T, rdb *redis.Client) {
	t.Helper()
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game.events", "result-workers", "$").Err()
}

// addAndReadMessages adds messages to the game.events stream, reads them via
// XReadGroup (so they enter the PEL), and returns the messages.
func addAndReadMessages(t *testing.T, rdb *redis.Client, payloads []string) []redis.XMessage {
	t.Helper()
	ctx := context.Background()

	ensureResultWorkerGroup(t, rdb)

	for _, p := range payloads {
		if err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "game.events",
			Values: map[string]interface{}{"payload": p},
		}).Err(); err != nil {
			t.Fatalf("XAdd: %v", err)
		}
	}

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    "result-workers",
		Consumer: "result-worker-1",
		Streams:  []string{"game.events", ">"},
		Count:    int64(len(payloads)),
		Block:    2 * time.Second,
	}).Result()
	if err != nil && err != redis.Nil {
		t.Fatalf("XReadGroup: %v", err)
	}

	var messages []redis.XMessage
	for _, stream := range streams {
		messages = append(messages, stream.Messages...)
	}
	return messages
}

// getPendingCount returns the number of pending messages in the consumer group.
func getPendingCount(t *testing.T, rdb *redis.Client) int64 {
	t.Helper()
	ctx := context.Background()
	ensureResultWorkerGroup(t, rdb)
	result, err := rdb.XPending(ctx, "game.events", "result-workers").Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	return result.Count
}

// getGameSessionStatus returns the status of the game session.
func getGameSessionStatus(t *testing.T, pool *pgxpool.Pool, sessionID string) string {
	t.Helper()
	ctx := context.Background()
	var status string
	err := pool.QueryRow(ctx, `SELECT status FROM game_sessions WHERE id = $1`, sessionID).Scan(&status)
	if err != nil {
		t.Fatalf("query game session status: %v", err)
	}
	return status
}

// getGameResultsCount returns the number of game_results rows for a session.
func getGameResultsCount(t *testing.T, pool *pgxpool.Pool, sessionID string) int {
	t.Helper()
	ctx := context.Background()
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM game_results WHERE session_id = $1`, sessionID).Scan(&count)
	if err != nil {
		t.Fatalf("query game results count: %v", err)
	}
	return count
}

// ─── Integration tests (testcontainers Redis + PostgreSQL) ───────────

// TestProcessBatch_NormalProcessing verifies that valid payloads are processed:
// game session updated, results inserted, and messages acked.
func TestProcessBatch_NormalProcessing(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, sessionID := setupGameTestData(t, pool)

	endedAt := time.Now().UnixMilli()
	payload := makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, endedAt)

	messages := addAndReadMessages(t, rdb, []string{payload})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// Verify game session was updated
	if status := getGameSessionStatus(t, pool, sessionID); status != "ended" {
		t.Errorf("game session status = %s, want ended", status)
	}

	// Verify game results were inserted
	if count := getGameResultsCount(t, pool, sessionID); count != 1 {
		t.Errorf("game results count = %d, want 1", count)
	}

	// Verify message was acked
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0", pending)
	}
}

// TestProcessBatch_MalformedPayload_NonString verifies that a message with a
// non-string payload field is acked without DB inserts.
func TestProcessBatch_MalformedPayload_NonString(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, sessionID := setupGameTestData(t, pool)

	// Add a valid message to get it into the PEL, then replace values
	messages := addAndReadMessages(t, rdb, []string{makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// Replace payload with non-string value
	messages[0].Values = map[string]interface{}{"payload": 12345}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// Verify no DB inserts (game session should still be 'active')
	if status := getGameSessionStatus(t, pool, sessionID); status != "active" {
		t.Errorf("game session status = %s, want active (no update should occur)", status)
	}

	// Verify message was acked (XAck uses message ID, which is in PEL)
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0 (invalid payload should be acked)", pending)
	}
}

// TestProcessBatch_InvalidJSON verifies that a message with invalid JSON payload
// is acked without DB inserts.
func TestProcessBatch_InvalidJSON(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, sessionID := setupGameTestData(t, pool)

	// Add a valid message to get it into the PEL, then replace with invalid JSON
	messages := addAndReadMessages(t, rdb, []string{makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// Replace with invalid JSON
	messages[0].Values = map[string]interface{}{"payload": "{invalid json}"}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// Verify no DB inserts
	if status := getGameSessionStatus(t, pool, sessionID); status != "active" {
		t.Errorf("game session status = %s, want active (no update should occur)", status)
	}

	// Verify message was acked
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0 (invalid JSON should be acked)", pending)
	}
}

// TestProcessBatch_DatabaseFailure_Rollback verifies FK violation on game_results
// does not ACK the message and does not commit partial rows.
func TestProcessBatch_DatabaseFailure_Rollback(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	_, sessionID := setupGameTestData(t, pool)

	nonExistentUserID := uuid.NewString()
	payload := makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: nonExistentUserID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())

	messages := addAndReadMessages(t, rdb, []string{payload})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	if pending := getPendingCount(t, rdb); pending != 1 {
		t.Errorf("pending count = %d, want 1 (FK failure should not ack)", pending)
	}
	if status := getGameSessionStatus(t, pool, sessionID); status != "active" {
		t.Errorf("game session status = %s, want active (transaction rolled back)", status)
	}
	if count := getGameResultsCount(t, pool, sessionID); count != 0 {
		t.Errorf("game results count = %d, want 0", count)
	}
}

// TestProcessBatch_DatabaseFailure_InvalidUUID verifies invalid GameID is acked as poison.
func TestProcessBatch_DatabaseFailure_InvalidUUID(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	payload := makeGameResultPayload("not-a-valid-uuid", 100, []PlayerGameResult{}, time.Now().UnixMilli())

	messages := addAndReadMessages(t, rdb, []string{payload})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0 (invalid payload acked as poison)", pending)
	}
}

// TestProcessBatch_Idempotency verifies that processing the same payload twice
// does not create duplicate game_results rows (UUID v5 + ON CONFLICT DO NOTHING).
func TestProcessBatch_Idempotency(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, sessionID := setupGameTestData(t, pool)

	endedAt := time.Now().UnixMilli()
	results := []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}
	payload := makeGameResultPayload(sessionID, 100, results, endedAt)

	// First processing
	messages1 := addAndReadMessages(t, rdb, []string{payload})
	if len(messages1) != 1 {
		t.Fatalf("expected 1 message on first read, got %d", len(messages1))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages1)

	if count := getGameResultsCount(t, pool, sessionID); count != 1 {
		t.Fatalf("after first processing: game results count = %d, want 1", count)
	}

	// Second processing (simulates retry/redelivery of the same payload)
	messages2 := addAndReadMessages(t, rdb, []string{payload})
	if len(messages2) != 1 {
		t.Fatalf("expected 1 message on second read, got %d", len(messages2))
	}

	w.processBatch(context.Background(), messages2)

	// Verify no duplicate insert (ON CONFLICT DO NOTHING)
	if count := getGameResultsCount(t, pool, sessionID); count != 1 {
		t.Errorf("after second processing: game results count = %d, want 1 (idempotency)", count)
	}

	// Both messages should be acked
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0 (all messages should be acked)", pending)
	}
}

// TestProcessBatch_EmptyBatch verifies that processBatch handles an empty message
// slice without errors (Begin + Commit with no operations).
func TestProcessBatch_EmptyBatch(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	w := NewGameResultWorker(rdb, pool)

	// Should not panic or error
	w.processBatch(context.Background(), []redis.XMessage{})

	// No messages to ack
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0", pending)
	}
}

// TestProcessBatch_MixedValidInvalid verifies that a batch with both valid and
// invalid messages processes correctly: invalid messages are acked individually,
// valid messages are processed and acked after commit.
func TestProcessBatch_MixedValidInvalid(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, sessionID := setupGameTestData(t, pool)

	validPayload := makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())

	// Add two messages: one valid, one with a placeholder (will be modified)
	messages := addAndReadMessages(t, rdb, []string{validPayload, validPayload})
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Make the second message invalid (non-string payload)
	messages[1].Values = map[string]interface{}{"payload": 999}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// Verify valid message was processed
	if status := getGameSessionStatus(t, pool, sessionID); status != "ended" {
		t.Errorf("game session status = %s, want ended", status)
	}
	if count := getGameResultsCount(t, pool, sessionID); count != 1 {
		t.Errorf("game results count = %d, want 1", count)
	}

	// Both messages should be acked
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0 (all messages should be acked)", pending)
	}
}

// TestProcessBatch_MultipleResults verifies processing a payload with multiple
// player results.
func TestProcessBatch_MultipleResults(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID1, sessionID := setupGameTestData(t, pool)

	// Create a second user
	userID2 := uuid.NewString()
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, nickname, palette, created_at) VALUES ($1, $2, $3, $4, $5)`,
		userID2, fmt.Sprintf("test2-%s@example.com", userID2[:8]), "TestUser2", 1, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}

	payload := makeGameResultPayload(sessionID, 200, []PlayerGameResult{
		{UserID: userID1, ScoreContribution: 120, TapsCount: 20},
		{UserID: userID2, ScoreContribution: 80, TapsCount: 15},
	}, time.Now().UnixMilli())

	messages := addAndReadMessages(t, rdb, []string{payload})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// Verify both results were inserted
	if count := getGameResultsCount(t, pool, sessionID); count != 2 {
		t.Errorf("game results count = %d, want 2", count)
	}

	// Verify message was acked
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0", pending)
	}
}

// TestProcessBatch_XAckAfterCommit verifies that XAck is called only after
// a successful commit (at-least-once semantics).
func TestProcessBatch_XAckAfterCommit(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, sessionID := setupGameTestData(t, pool)

	payload := makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())

	messages := addAndReadMessages(t, rdb, []string{payload})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// After successful commit, the message should be acked
	if pending := getPendingCount(t, rdb); pending != 0 {
		t.Errorf("pending count = %d, want 0 (XAck should be called after commit)", pending)
	}

	// Verify the data was actually committed
	if status := getGameSessionStatus(t, pool, sessionID); status != "ended" {
		t.Errorf("game session status = %s, want ended (commit should have succeeded)", status)
	}
	if count := getGameResultsCount(t, pool, sessionID); count != 1 {
		t.Errorf("game results count = %d, want 1 (commit should have succeeded)", count)
	}
}

// TestStart_ContextCancellation verifies that Start processes messages and
// exits cleanly on context cancellation, flushing any remaining batch.
func TestStart_ContextCancellation(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, sessionID := setupGameTestData(t, pool)

	// Create consumer group and enqueue a message before starting the worker
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game.events", "result-workers", "$").Err()

	payload := makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())

	if err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "game.events",
		Values: map[string]interface{}{"payload": payload},
	}).Err(); err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	w := NewGameResultWorker(rdb, pool)

	workerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(workerCtx)
		close(done)
	}()

	// Wait for the message to be processed (poll game_sessions status)
	deadline := time.Now().Add(10 * time.Second)
	processed := false
	for time.Now().Before(deadline) {
		if status := getGameSessionStatusQuiet(pool, sessionID); status == "ended" {
			processed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !processed {
		t.Fatal("message was not processed within 10s")
	}

	// Cancel context and verify worker exits
	cancel()
	select {
	case <-done:
		// worker exited successfully
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not exit within 5s after context cancellation")
	}

	// Verify the message was processed
	if count := getGameResultsCount(t, pool, sessionID); count != 1 {
		t.Errorf("game results count = %d, want 1", count)
	}
}

// getGameSessionStatusQuiet returns the game session status without failing on error.
func getGameSessionStatusQuiet(pool *pgxpool.Pool, sessionID string) string {
	ctx := context.Background()
	var status string
	err := pool.QueryRow(ctx, `SELECT status FROM game_sessions WHERE id = $1`, sessionID).Scan(&status)
	if err != nil {
		return ""
	}
	return status
}
