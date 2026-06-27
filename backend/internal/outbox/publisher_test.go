//go:build integration

package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/testutil"
)

// testEnv holds testcontainers resources for a single test.
type testEnv struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	pool := testutil.SetupPostgresPoolMigrated(t)
	rdb, _ := testutil.SetupRedisClient(t)
	return &testEnv{pool: pool, rdb: rdb}
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
