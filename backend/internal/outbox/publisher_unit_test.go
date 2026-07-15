package outbox

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
)

func TestNewPublisher_EnvConfig(t *testing.T) {
	if err := os.Setenv("OUTBOX_BATCH_SIZE", "25"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("OUTBOX_POLL_INTERVAL_MS", "500"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("OUTBOX_BATCH_SIZE")
		_ = os.Unsetenv("OUTBOX_POLL_INTERVAL_MS")
	})

	pub := NewPublisher(nil, nil)
	if pub.batchSize != 25 {
		t.Errorf("batchSize = %d, want 25", pub.batchSize)
	}
	if pub.interval != 500*time.Millisecond {
		t.Errorf("interval = %v, want 500ms", pub.interval)
	}
}

func TestNewPublisher_Defaults(t *testing.T) {
	_ = os.Unsetenv("OUTBOX_BATCH_SIZE")
	_ = os.Unsetenv("OUTBOX_POLL_INTERVAL_MS")

	pub := NewPublisher(nil, nil)
	if pub.batchSize != 100 {
		t.Errorf("batchSize = %d, want 100", pub.batchSize)
	}
	if pub.interval != time.Second {
		t.Errorf("interval = %v, want 1s", pub.interval)
	}
}

func TestNewPublisher_InvalidEnvIgnored(t *testing.T) {
	if err := os.Setenv("OUTBOX_BATCH_SIZE", "bad"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("OUTBOX_POLL_INTERVAL_MS", "-1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("OUTBOX_BATCH_SIZE")
		_ = os.Unsetenv("OUTBOX_POLL_INTERVAL_MS")
	})

	pub := NewPublisher(nil, nil)
	if pub.batchSize != 100 || pub.interval != time.Second {
		t.Errorf("batchSize=%d interval=%v, want defaults", pub.batchSize, pub.interval)
	}
}

func TestPublisher_Start_CancelledContext(t *testing.T) {
	pub := &Publisher{interval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		pub.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestPublisher_Start_RunsPublishCycle(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "payload", "created_at"}))
	mock.ExpectRollback()

	pub := &Publisher{db: mock, interval: 20 * time.Millisecond, batchSize: 10}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	pub.Start(ctx)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPublisher_publishBatch_BeginError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	pub := &Publisher{db: mock, rdb: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})}
	pub.publishBatch(context.Background())
}

func TestPublisher_publishBatch_EmptyBatch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "payload", "created_at"}))
	mock.ExpectRollback()

	pub := &Publisher{db: mock, batchSize: 10}
	pub.publishBatch(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPublisher_publishBatch_RedisPipelineError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "payload", "created_at"}).
			AddRow(int64(1), "room", "room-1", []byte(`{"e":1}`), int64(1000)))
	mock.ExpectRollback()

	brokenRdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = brokenRdb.Close() })

	pub := &Publisher{db: mock, rdb: brokenRdb, batchSize: 10}
	pub.publishBatch(context.Background())
}

func TestPublisher_publishBatch_NilDBPanics(t *testing.T) {
	pub := &Publisher{db: nil, rdb: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic with nil db")
		}
	}()
	pub.publishBatch(context.Background())
}

func TestPublisher_publishBatch_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "payload", "created_at"}).
			AddRow(int64(7), "game", "game-1", []byte(`{"started":true}`), int64(500)))
	mock.ExpectExec("UPDATE outbox_events SET processed_at").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectCommit()

	pub := &Publisher{db: mock, rdb: rdb, batchSize: 10}
	pub.publishBatch(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if n, _ := rdb.XLen(context.Background(), "game.events").Result(); n != 1 {
		t.Fatalf("expected 1 message in game.events, got %d", n)
	}
}

func TestPublisher_publishBatch_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnError(errors.New("query failed"))
	mock.ExpectRollback()

	pub := &Publisher{db: mock, batchSize: 10}
	pub.publishBatch(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPublisher_publishBatch_ScanError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "payload", "created_at"}).
			AddRow("bad-id", "game", "game-1", []byte(`{}`), int64(500)))
	mock.ExpectRollback()

	pub := &Publisher{db: mock, batchSize: 10}
	pub.publishBatch(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPublisher_publishBatch_MarkProcessedError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "payload", "created_at"}).
			AddRow(int64(3), "room", "room-1", []byte(`{"e":1}`), int64(1000)))
	mock.ExpectExec("UPDATE outbox_events SET processed_at").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	pub := &Publisher{db: mock, rdb: rdb, batchSize: 10}
	pub.publishBatch(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPublisher_publishBatch_CommitError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, aggregate_type, aggregate_id, payload, created_at").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "payload", "created_at"}).
			AddRow(int64(9), "game", "game-9", []byte(`{"done":true}`), int64(2000)))
	mock.ExpectExec("UPDATE outbox_events SET processed_at").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
	mock.ExpectRollback()

	pub := &Publisher{db: mock, rdb: rdb, batchSize: 10}
	pub.publishBatch(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
