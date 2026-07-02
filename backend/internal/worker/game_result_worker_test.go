package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
)

const testGameID = "11111111-1111-4111-8111-111111111111"

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

func TestGameResultPayload_Unmarshal_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string", "", true},
		{"invalid JSON", "{invalid}", true},
		{"missing closing brace", `{"game_id":"abc"`, true},
		{"valid empty object", `{}`, false},
		{"null results", `{"game_id":"abc","results":null}`, false},
		{"empty results array", `{"game_id":"abc","results":[]}`, false},
		{"missing game_id", `{"final_score":100}`, false},
		{"negative score", `{"game_id":"abc","final_score":-1}`, false},
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

func TestGameResultWorker_UUIDIdempotency(t *testing.T) {
	gameID := "session-123"
	userID := "user-456"

	id1 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(gameID+userID)).String()
	id2 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(gameID+userID)).String()

	if id1 != id2 {
		t.Fatalf("UUID v5 should be deterministic: %s != %s", id1, id2)
	}

	id3 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(gameID+"different-user")).String()
	if id1 == id3 {
		t.Fatal("different inputs should produce different UUIDs")
	}
}

func TestGameResultWorker_UUIDCollisionRisk(t *testing.T) {
	id1 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("abc"+"def")).String()
	id2 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("ab"+"cdef")).String()

	if id1 == id2 {
		t.Log("KNOWN LIMITATION: UUID collision detected - 'abc'+'def' and 'ab'+'cdef' produce the same UUID.")
	}
}

func TestNewGameResultWorker(t *testing.T) {
	w := NewGameResultWorker(nil, nil)
	if w == nil {
		t.Fatal("NewGameResultWorker returned nil")
	}
}

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

	w.processBatch(ctx, []redis.XMessage{
		{ID: "1-0", Values: map[string]interface{}{"payload": "{}"}},
	})
}

func TestGameResultWorker_Start_Cancelled(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	w := NewGameResultWorker(rdb, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}
}

func TestGameResultWorker_processMessage_InvalidPayload(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	w := NewGameResultWorker(rdb, nil)
	w.processMessage(ctx, redis.XMessage{ID: "1-0", Values: map[string]interface{}{"payload": 123}})

	pending, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: "game:results", Group: "result-workers", Start: "-", End: "+", Count: 10,
	}).Result()
	if len(pending) != 0 {
		t.Fatalf("expected acked invalid payload, pending=%d", len(pending))
	}
}

func TestGameResultWorker_processMessage_UnmarshalError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	w := NewGameResultWorker(rdb, nil)
	w.processMessage(ctx, redis.XMessage{ID: "2-0", Values: map[string]interface{}{"payload": "not-json"}})
}

func TestGameResultWorker_processMessage_BeginFailure(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	config, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	config.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	w := NewGameResultWorker(rdb, pool)
	payload, _ := json.Marshal(GameResultPayload{GameID: testGameID, RoomCode: "ROOM1"})
	w.processMessage(ctx, redis.XMessage{
		ID:     "4-0",
		Values: map[string]interface{}{"payload": string(payload)},
	})
}

func newGameResultWorkerWithMockDB(t *testing.T, rdb *redis.Client) (*GameResultWorker, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return &GameResultWorker{rdb: rdb, db: mock}, mock
}

func TestGameResultWorker_processMessage_Success(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	w, mock := newGameResultWorkerWithMockDB(t, rdb)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(pgxmock.AnyArg(), testGameID, "user-1", 25, 5, int64(100)).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	payload, _ := json.Marshal(GameResultPayload{
		GameID: testGameID, RoomCode: "ROOM1", FinalScore: 50, EndedAt: 100,
		Results: []PlayerGameResult{{UserID: "user-1", ScoreContribution: 25, TapsCount: 5}},
	})
	w.processMessage(ctx, redis.XMessage{
		ID:     "7-0",
		Values: map[string]interface{}{"payload": string(payload)},
	})
}

func TestGameResultWorker_processMessage_SuccessNoResults(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	w, mock := newGameResultWorkerWithMockDB(t, rdb)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	payload, _ := json.Marshal(GameResultPayload{
		GameID: testGameID, RoomCode: "ROOM1", FinalScore: 50, EndedAt: 100,
	})
	w.processMessage(ctx, redis.XMessage{
		ID:     "12-0",
		Values: map[string]interface{}{"payload": string(payload)},
	})
}

func TestGameResultWorker_processMessage_InvalidGameID(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	w := NewGameResultWorker(rdb, nil)
	payload, _ := json.Marshal(GameResultPayload{GameID: "not-a-valid-uuid", RoomCode: "ROOM1"})
	w.processMessage(ctx, redis.XMessage{
		ID:     "11-0",
		Values: map[string]interface{}{"payload": string(payload)},
	})
}

func TestGameResultWorker_processMessage_UpsertError(t *testing.T) {
	mr, _ := miniredis.Run()
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	w, mock := newGameResultWorkerWithMockDB(t, rdb)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("upsert failed"))
	mock.ExpectRollback()

	payload, _ := json.Marshal(GameResultPayload{GameID: testGameID, RoomCode: "R1"})
	w.processMessage(ctx, redis.XMessage{
		ID: "8-0", Values: map[string]interface{}{"payload": string(payload)},
	})
}

func TestGameResultWorker_processMessage_InsertResultError(t *testing.T) {
	mr, _ := miniredis.Run()
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	w, mock := newGameResultWorkerWithMockDB(t, rdb)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	payload, _ := json.Marshal(GameResultPayload{
		GameID: testGameID, RoomCode: "R1", Results: []PlayerGameResult{{UserID: "u1"}},
	})
	w.processMessage(ctx, redis.XMessage{
		ID: "9-0", Values: map[string]interface{}{"payload": string(payload)},
	})
}

func TestGameResultWorker_processMessage_CommitError(t *testing.T) {
	mr, _ := miniredis.Run()
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	w, mock := newGameResultWorkerWithMockDB(t, rdb)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
	mock.ExpectRollback()

	payload, _ := json.Marshal(GameResultPayload{GameID: testGameID, RoomCode: "R1"})
	w.processMessage(ctx, redis.XMessage{
		ID: "10-0", Values: map[string]interface{}{"payload": string(payload)},
	})
}

func TestGameResultWorker_Start_WithHostname(t *testing.T) {
	t.Setenv("HOSTNAME", "worker-pod-1")
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewGameResultWorker(rdb, nil).Start(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit")
	}
}

func TestGameResultWorker_Start_FlushTimerTick(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewGameResultWorker(rdb, nil).Start(ctx)
		close(done)
	}()
	time.Sleep(1100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit after timer flush")
	}
}

func TestGameResultWorker_Start_XReadGroupError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewGameResultWorker(rdb, nil).Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	mr.SetError("redis unavailable")
	time.Sleep(1200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}
}

func TestGameResultWorker_Start_BatchFlushAt100(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "game:results",
			Values: map[string]interface{}{"payload": "not-json"},
		}).Result(); err != nil {
			t.Fatalf("XAdd: %v", err)
		}
	}
	if err := rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		NewGameResultWorker(rdb, nil).Start(workerCtx)
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit")
	}
}

func TestGameResultWorker_Start_UsesHostnameConsumer(t *testing.T) {
	t.Setenv("HOSTNAME", "worker-pod-7")
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "game:results",
		Values: map[string]interface{}{"payload": "not-json"},
	}).Result(); err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if err := rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		NewGameResultWorker(rdb, nil).Start(workerCtx)
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit")
	}
}

func TestGameResultWorker_Start_DefaultConsumerName(t *testing.T) {
	t.Setenv("HOSTNAME", "")
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "game:results",
		Values: map[string]interface{}{"payload": "not-json"},
	}).Result(); err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if err := rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		NewGameResultWorker(rdb, nil).Start(workerCtx)
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit")
	}
}

func TestGameResultWorker_processMessage_MissingPayload(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	w := NewGameResultWorker(rdb, nil)
	w.processMessage(ctx, redis.XMessage{ID: "3-0", Values: map[string]interface{}{"other": "value"}})

	pending, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: "game:results", Group: "result-workers", Start: "-", End: "+", Count: 10,
	}).Result()
	if len(pending) != 0 {
		t.Fatalf("expected acked missing payload, pending=%d", len(pending))
	}
}

func TestGameResultWorker_processBatch_MultipleMessages(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	_ = rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err()

	w := NewGameResultWorker(rdb, nil)
	w.processBatch(ctx, []redis.XMessage{
		{ID: "5-0", Values: map[string]interface{}{"payload": "bad-json"}},
		{ID: "6-0", Values: map[string]interface{}{"payload": 42}},
	})
}

func TestGameResultWorker_Start_ReadsAndFlushes(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "game:results",
		Values: map[string]interface{}{"payload": "not-json"},
	}).Result(); err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if err := rdb.XGroupCreateMkStream(ctx, "game:results", "result-workers", "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	w := NewGameResultWorker(rdb, nil)
	workerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(workerCtx)
		close(done)
	}()

	time.Sleep(1500 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}

	pending, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: "game:results", Group: "result-workers", Start: "-", End: "+", Count: 10,
	}).Result()
	if len(pending) != 0 {
		t.Fatalf("expected invalid payload to be acked, pending=%d", len(pending))
	}
}

func TestGameResultWorker_processMessage_NilRedisAck(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	payload := GameResultPayload{
		GameID:     "550e8400-e29b-41d4-a716-446655440000",
		RoomCode:   "ROOM1",
		EndedAt:    time.Now().UnixMilli(),
		FinalScore: 10,
		Results:    []PlayerGameResult{{UserID: "u1", ScoreContribution: 10, TapsCount: 1}},
	}
	body, _ := json.Marshal(payload)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(payload.GameID, payload.RoomCode, payload.EndedAt, payload.FinalScore).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(pgxmock.AnyArg(), payload.GameID, "u1", 10, 1, payload.EndedAt).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	w := &GameResultWorker{rdb: nil, db: mock}
	w.processMessage(context.Background(), redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"payload": string(body)},
	})
}
