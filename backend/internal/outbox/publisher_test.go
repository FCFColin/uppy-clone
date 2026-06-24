package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ─── Test helpers ────────────────────────────────────────────────────

// testEnv holds testcontainers resources for a single test.
type testEnv struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool := startTestPostgres(t, ctx)
	rdb := startTestRedis(t, ctx)

	return &testEnv{pool: pool, rdb: rdb}
}

// startTestPostgres starts a PostgreSQL testcontainer, creates a connection pool,
// and applies the outbox_events migration.
func startTestPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
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
	t.Cleanup(func() { pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Apply outbox_events migration
	migPath := filepath.Join(migrationsDir(t), "000007_create_outbox_events.up.sql")
	sql, err := os.ReadFile(migPath) //nolint:gosec // test path is controlled
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	return pool
}

// startTestRedis starts a Redis testcontainer and returns a connected client.
func startTestRedis(t *testing.T, ctx context.Context) *redis.Client {
	t.Helper()
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

// migrationsDir resolves the absolute path to backend/migrations.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// This file is at backend/internal/outbox/publisher_test.go
	// migrations are at backend/migrations/
	dir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	return abs
}

// insertOutboxEvent inserts a test event into outbox_events.
func insertOutboxEvent(ctx context.Context, pool *pgxpool.Pool, aggType, aggID string, payload []byte) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, payload) VALUES ($1, $2, $3) RETURNING id`,
		aggType, aggID, payload).Scan(&id)
	return id, err
}

// countUnprocessed returns the number of unprocessed outbox events.
func countUnprocessed(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var count int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbox_events WHERE processed_at IS NULL`).Scan(&count)
	return count, err
}

// ─── Tests ───────────────────────────────────────────────────────────

// TestPublisher_ProcessesEvents verifies the publisher publishes outbox events
// to Redis Streams and marks them as processed.
func TestPublisher_ProcessesEvents(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	pub := NewPublisher(env.pool, env.rdb)

	// Insert 3 events
	events := []struct {
		aggType string
		aggID   string
		payload []byte
	}{
		{"room", "room-1", []byte(`{"event":"created"}`)},
		{"room", "room-2", []byte(`{"event":"created"}`)},
		{"game", "game-1", []byte(`{"event":"started"}`)},
	}
	for _, e := range events {
		if _, err := insertOutboxEvent(ctx, env.pool, e.aggType, e.aggID, e.payload); err != nil {
			t.Fatalf("insert outbox event: %v", err)
		}
	}

	// Run one publish cycle
	pub.publishBatch(ctx)

	// All events should be marked as processed
	unprocessed, err := countUnprocessed(ctx, env.pool)
	if err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if unprocessed != 0 {
		t.Fatalf("expected 0 unprocessed events, got %d", unprocessed)
	}

	// Verify events were published to Redis Streams
	// room.events stream should have 2 messages
	roomMsgs, err := env.rdb.XLen(ctx, "room.events").Result()
	if err != nil {
		t.Fatalf("XLen room.events: %v", err)
	}
	if roomMsgs != 2 {
		t.Fatalf("expected 2 messages in room.events, got %d", roomMsgs)
	}

	// game.events stream should have 1 message
	gameMsgs, err := env.rdb.XLen(ctx, "game.events").Result()
	if err != nil {
		t.Fatalf("XLen game.events: %v", err)
	}
	if gameMsgs != 1 {
		t.Fatalf("expected 1 message in game.events, got %d", gameMsgs)
	}
}

// TestPublisher_EmptyOutbox verifies the publisher handles an empty outbox gracefully.
func TestPublisher_EmptyOutbox(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	pub := NewPublisher(env.pool, env.rdb)

	// Run publishBatch on empty outbox — should not error or panic
	pub.publishBatch(ctx)

	unprocessed, err := countUnprocessed(ctx, env.pool)
	if err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if unprocessed != 0 {
		t.Fatalf("expected 0 unprocessed events, got %d", unprocessed)
	}
}

// TestPublisher_ContinuesAfterError verifies the publisher continues processing
// after an error (e.g., Redis temporarily unavailable). Events that fail to
// publish remain unprocessed and are retried on the next cycle.
func TestPublisher_ContinuesAfterError(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Insert events
	for i := 0; i < 3; i++ {
		if _, err := insertOutboxEvent(ctx, env.pool, "room",
			fmt.Sprintf("room-%d", i), []byte(`{"event":"created"}`)); err != nil {
			t.Fatalf("insert outbox event: %v", err)
		}
	}

	// Create a publisher with a broken Redis client (wrong address)
	brokenRdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // unreachable
	defer func() { _ = brokenRdb.Close() }()
	pub := NewPublisher(env.pool, brokenRdb)

	// publishBatch should not panic — it logs errors and continues
	pub.publishBatch(ctx)

	// All events should remain unprocessed (Redis was unreachable)
	unprocessed, err := countUnprocessed(ctx, env.pool)
	if err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if unprocessed != 3 {
		t.Fatalf("expected 3 unprocessed events after Redis failure, got %d", unprocessed)
	}

	// Now switch to the working Redis and verify events get published
	pub2 := NewPublisher(env.pool, env.rdb)
	pub2.publishBatch(ctx)

	unprocessed, err = countUnprocessed(ctx, env.pool)
	if err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if unprocessed != 0 {
		t.Fatalf("expected 0 unprocessed events after retry, got %d", unprocessed)
	}
}

// TestPublisher_MarksProcessed verifies processed_at is set after publishing.
func TestPublisher_MarksProcessed(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	pub := NewPublisher(env.pool, env.rdb)

	id, err := insertOutboxEvent(ctx, env.pool, "room", "room-x", []byte(`{"event":"test"}`))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	pub.publishBatch(ctx)

	var processedAt *int64
	err = env.pool.QueryRow(ctx,
		`SELECT processed_at FROM outbox_events WHERE id = $1`, id).Scan(&processedAt)
	if err != nil {
		t.Fatalf("query processed_at: %v", err)
	}
	if processedAt == nil {
		t.Fatal("expected processed_at to be non-nil after publishing")
	}
	if *processedAt == 0 {
		t.Fatal("expected processed_at to be non-zero")
	}
}

// TestPublisher_PayloadContent verifies the published payload matches the outbox event.
func TestPublisher_PayloadContent(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	pub := NewPublisher(env.pool, env.rdb)

	payload := `{"event":"created","room":"test-room"}`
	if _, err := insertOutboxEvent(ctx, env.pool, "room", "room-payload", []byte(payload)); err != nil {
		t.Fatalf("insert: %v", err)
	}

	pub.publishBatch(ctx)

	// Read the message from Redis Stream
	msgs, err := env.rdb.XRange(ctx, "room.events", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	aggID, ok := msgs[0].Values["aggregate_id"].(string)
	if !ok || aggID != "room-payload" {
		t.Fatalf("expected aggregate_id=room-payload, got %v", msgs[0].Values["aggregate_id"])
	}

	// PostgreSQL JSONB normalizes JSON (reorders keys, adds spaces),
	// so compare semantically by parsing both sides.
	pubPayload, ok := msgs[0].Values["payload"].(string)
	if !ok {
		t.Fatal("missing payload in published message")
	}

	var expected, actual map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &expected); err != nil {
		t.Fatalf("unmarshal expected payload: %v", err)
	}
	if err := json.Unmarshal([]byte(pubPayload), &actual); err != nil {
		t.Fatalf("unmarshal published payload: %v", err)
	}
	if expected["event"] != actual["event"] || expected["room"] != actual["room"] {
		t.Fatalf("payload mismatch: expected %v, got %v", expected, actual)
	}
}
