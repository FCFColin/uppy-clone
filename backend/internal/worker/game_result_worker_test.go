package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// ─── Pure unit tests (no Docker required) ────────────────────────────

// TestGameResultPayload_Unmarshal verifies JSON unmarshaling of valid payloads.
func TestGameResultPayload_Unmarshal(t *testing.T) {
	payload := GameResultPayload{
		GameID:     "session-123",
		RoomCode:   "ROOM1",
		FinalScore: 100,
		Results: []PlayerGameResult{
			{UserID: "user-1", ScoreContribution: 50, TapsCount: 10},
			{UserID: "user-2", ScoreContribution: 50, TapsCount: 10},
		},
		EndedAt: 1700000000,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got GameResultPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.GameID != payload.GameID {
		t.Errorf("GameID = %s, want %s", got.GameID, payload.GameID)
	}
	if got.RoomCode != payload.RoomCode {
		t.Errorf("RoomCode = %s, want %s", got.RoomCode, payload.RoomCode)
	}
	if got.FinalScore != payload.FinalScore {
		t.Errorf("FinalScore = %d, want %d", got.FinalScore, payload.FinalScore)
	}
	if got.EndedAt != payload.EndedAt {
		t.Errorf("EndedAt = %d, want %d", got.EndedAt, payload.EndedAt)
	}
	if len(got.Results) != 2 {
		t.Fatalf("Results length = %d, want 2", len(got.Results))
	}
	if got.Results[0].UserID != "user-1" {
		t.Errorf("Results[0].UserID = %s, want user-1", got.Results[0].UserID)
	}
	if got.Results[0].ScoreContribution != 50 {
		t.Errorf("Results[0].ScoreContribution = %d, want 50", got.Results[0].ScoreContribution)
	}
	if got.Results[0].TapsCount != 10 {
		t.Errorf("Results[0].TapsCount = %d, want 10", got.Results[0].TapsCount)
	}
}

// TestGameResultPayload_Unmarshal_EdgeCases verifies unmarshaling edge cases.
func TestGameResultPayload_Unmarshal_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantField string
	}{
		{"empty string", "", true, ""},
		{"invalid JSON", "{invalid}", true, ""},
		{"missing closing brace", `{"game_id":"abc"`, true, ""},
		{"valid empty object", `{}`, false, ""},
		{"null results", `{"game_id":"abc","results":null}`, false, ""},
		{"empty results array", `{"game_id":"abc","results":[]}`, false, ""},
		{"missing game_id", `{"final_score":100}`, false, ""},
		{"negative score", `{"game_id":"abc","final_score":-1}`, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload GameResultPayload
			err := json.Unmarshal([]byte(tt.input), &payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestGameResultWorker_UUIDIdempotency verifies that UUID v5 generates the same
// ID for the same gameID + userID combination (idempotency guarantee).
// This is the core mechanism that prevents duplicate inserts on retry.
func TestGameResultWorker_UUIDIdempotency(t *testing.T) {
	gameID := "session-123"
	userID := "user-456"

	id1 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(gameID+userID)).String()
	id2 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(gameID+userID)).String()

	if id1 != id2 {
		t.Fatalf("UUID v5 should be deterministic: %s != %s", id1, id2)
	}

	// Different input should produce different UUID
	id3 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(gameID+"different-user")).String()
	if id1 == id3 {
		t.Fatal("different inputs should produce different UUIDs")
	}
}

// TestGameResultWorker_UUIDCollisionRisk documents a potential UUID collision
// risk: string concatenation of GameID+UserID can produce the same input for
// different pairs. This is a known design limitation.
func TestGameResultWorker_UUIDCollisionRisk(t *testing.T) {
	// These different (GameID, UserID) pairs produce the same concatenation:
	// "abc" + "def" == "ab" + "cdef" == "abcdef"
	id1 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("abc"+"def")).String()
	id2 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("ab"+"cdef")).String()

	if id1 == id2 {
		t.Log("KNOWN LIMITATION: UUID collision detected - 'abc'+'def' and 'ab'+'cdef' produce the same UUID.")
		t.Log("In practice, GameIDs are UUIDs so prefix overlap is unlikely, but a separator (e.g., ':') would be safer.")
	}
}

// TestNewGameResultWorker verifies the constructor returns non-nil.
func TestNewGameResultWorker(t *testing.T) {
	w := NewGameResultWorker(nil, nil)
	if w == nil {
		t.Fatal("NewGameResultWorker returned nil")
	}
}

// TestProcessBatch_BeginFailure verifies that processBatch handles Begin failure
// gracefully (no panic, returns without processing). Uses an invalid PG pool
// — no Docker required.
func TestProcessBatch_BeginFailure(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	config.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	w := &GameResultWorker{rdb: nil, db: pool}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Should return without panic; Begin fails so XAck is never called (rdb is nil)
	w.processBatch(ctx, []redis.XMessage{
		{ID: "1-0", Values: map[string]interface{}{"payload": "{}"}},
	})
}

// ─── Integration test helpers ────────────────────────────────────────

// setupPostgresPool starts a PostgreSQL testcontainer and returns a connected pool.
// Skips the test if Docker is unavailable or in short mode.
func setupPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Skipf("skipping: postgres container unavailable (Docker not running?): %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	timeouts := config.TimeoutConfig{
		PGConnectTimeout: 10 * time.Second,
		PGQueryTimeout:   10 * time.Second,
		PGRequestTimeout: 30 * time.Second,
	}

	db, err := store.NewPostgresStore(connStr, timeouts)
	if err != nil {
		t.Fatalf("failed to create PostgresStore: %v", err)
	}

	migrationsPath := workerMigrationsDir(t)
	if err := db.RunMigrations(migrationsPath); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	pool := db.Pool()
	t.Cleanup(func() { pool.Close() })
	return pool
}

// workerMigrationsDir resolves the absolute path to the backend/migrations directory.
func workerMigrationsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// This file is at backend/internal/worker/game_result_worker_test.go
	// migrations are at backend/migrations/
	dir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	return abs
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

// makeGameResultPayload creates a JSON-encoded GameResultPayload string.
func makeGameResultPayload(gameID string, finalScore int, results []PlayerGameResult, endedAt int64) string {
	p := GameResultPayload{
		GameID:     gameID,
		RoomCode:   "ROOM1",
		FinalScore: finalScore,
		Results:    results,
		EndedAt:    endedAt,
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// addAndReadMessages adds messages to the game:results stream, reads them via
// XReadGroup (so they enter the PEL), and returns the messages.
func addAndReadMessages(t *testing.T, rdb *redis.Client, payloads []string) []redis.XMessage {
	t.Helper()
	ctx := context.Background()

	// Ensure consumer group exists
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "$").Err()

	for _, p := range payloads {
		if err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "game:results",
			Values: map[string]interface{}{"payload": p},
		}).Err(); err != nil {
			t.Fatalf("XAdd: %v", err)
		}
	}

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    "result-workers",
		Consumer: "result-worker-1",
		Streams:  []string{"game:results", ">"},
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
	result, err := rdb.XPending(ctx, "game:results", "result-workers").Result()
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

// setupRedisForWorker starts a Redis testcontainer and returns a connected client.
// Skips the test if Docker is unavailable or in short mode.
func setupRedisForWorker(t *testing.T) *redis.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	redisContainer, err := tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(10*time.Second)),
	)
	if err != nil {
		t.Skipf("skipping: redis container unavailable (Docker not running?): %v", err)
	}
	t.Cleanup(func() { _ = redisContainer.Terminate(ctx) })

	addr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("failed to get redis endpoint: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { rdb.Close() })

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	return rdb
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

// TestProcessBatch_DatabaseFailure_Rollback verifies that a DB failure during
// processing causes the entire batch to be rolled back, and messages are NOT acked.
func TestProcessBatch_DatabaseFailure_Rollback(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	userID, _ := setupGameTestData(t, pool)

	// Use a non-existent GameID (valid UUID format but not in DB).
	// The UPDATE on game_sessions succeeds (0 rows updated), but the INSERT
	// on game_results fails with FK violation (session_id doesn't exist).
	nonExistentSessionID := uuid.NewString()
	payload := makeGameResultPayload(nonExistentSessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())

	messages := addAndReadMessages(t, rdb, []string{payload})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// Verify message was NOT acked (will be retried for at-least-once semantics)
	if pending := getPendingCount(t, rdb); pending != 1 {
		t.Errorf("pending count = %d, want 1 (failed batch should not be acked)", pending)
	}
}

// TestProcessBatch_DatabaseFailure_InvalidUUID verifies that an invalid UUID
// as GameID causes the UPDATE to fail, triggering a rollback.
func TestProcessBatch_DatabaseFailure_InvalidUUID(t *testing.T) {
	pool := setupPostgresPool(t)
	rdb := setupRedisForWorker(t)

	// Use an invalid UUID as GameID — UPDATE will fail with syntax error
	payload := makeGameResultPayload("not-a-valid-uuid", 100, []PlayerGameResult{}, time.Now().UnixMilli())

	messages := addAndReadMessages(t, rdb, []string{payload})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	w := NewGameResultWorker(rdb, pool)
	w.processBatch(context.Background(), messages)

	// Verify message was NOT acked
	if pending := getPendingCount(t, rdb); pending != 1 {
		t.Errorf("pending count = %d, want 1 (DB failure should not ack)", pending)
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
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "$").Err()

	payload := makeGameResultPayload(sessionID, 100, []PlayerGameResult{
		{UserID: userID, ScoreContribution: 50, TapsCount: 10},
	}, time.Now().UnixMilli())

	if err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "game:results",
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
