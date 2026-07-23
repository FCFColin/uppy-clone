package audit

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sethvargo/go-retry"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func unreachableAuditPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
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
	return pool
}

type fakeAuditPool struct {
	execErr error
}

func (f *fakeAuditPool) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return pgconn.NewCommandTag("INSERT 1"), nil
}

// flakyAuditPool fails the first failN Exec calls with failErr, then succeeds.
// Used to verify retry behavior in writeToDB (v2-R-37).
type flakyAuditPool struct {
	calls   int32 // atomic
	failN   int32
	failErr error
}

func (f *flakyAuditPool) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	n := atomic.AddInt32(&f.calls, 1)
	if n <= f.failN {
		return pgconn.CommandTag{}, f.failErr
	}
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func testRetryPolicy() RetryPolicy {
	return RetryPolicy{
		DBRetry:        testDBRetry(),
		MaybeRetryable: testMaybeRetryable,
	}
}

func testDBRetry() retry.Backoff {
	b := retry.NewExponential(100 * time.Millisecond)
	return retry.WithMaxRetries(3, retry.WithJitter(50*time.Millisecond, b))
}

func testMaybeRetryable(err error) error {
	if err == nil {
		return nil
	}
	return retry.RetryableError(err)
}

func TestInitDBLogger_NilPoolNoOp(t *testing.T) {
	// Adversarial: nil pool must not panic or initialize dbLogger.
	old := dbLogger
	defer func() { dbLogger = old }()

	InitDBLogger(nil, "secret", RetryPolicy{})
	if dbLogger != nil {
		t.Fatal("InitDBLogger with nil pool should not initialize dbLogger")
	}
	CloseDBLogger()
}

func TestInitDBLogger_EmptySecretNoOp(t *testing.T) {
	old := dbLogger
	defer func() { dbLogger = old }()

	InitDBLogger(nil, "", RetryPolicy{})
	if dbLogger != nil {
		t.Fatal("InitDBLogger with empty secret should not initialize dbLogger")
	}
}

func TestInitDBLogger_ReplacesExisting(t *testing.T) {
	old := dbLogger
	defer func() {
		if dbLogger != nil {
			CloseDBLogger()
		}
		dbLogger = old
	}()

	pool := unreachableAuditPool(t)
	InitDBLogger(pool, "audit-secret-key-for-hmac-chain!!", testRetryPolicy())
	InitDBLogger(pool, "audit-secret-key-for-hmac-chain!!", testRetryPolicy())
	if dbLogger == nil {
		t.Fatal("expected dbLogger after replace init")
	}
}

func TestLog_StdoutOnlyWithoutDB(t *testing.T) {
	var buf bytes.Buffer
	old := auditLogger
	auditLogger = slog.New(slog.NewJSONHandler(&buf, nil))
	defer func() { auditLogger = old }()

	Log(context.Background(), AuditEntry{
		Action:   "test.action",
		ActorID:  "user-1",
		ActorIP:  "127.0.0.1",
		Resource: "test",
	})
	if !bytes.Contains(buf.Bytes(), []byte("test.action")) {
		t.Errorf("log output = %s", buf.String())
	}
}

func TestLog_AutoRequestID(t *testing.T) {
	var buf bytes.Buffer
	old := auditLogger
	auditLogger = slog.New(slog.NewJSONHandler(&buf, nil))
	defer func() { auditLogger = old }()

	ctx := context.WithValue(context.Background(), middleware.RequestIDKey, "req-123")
	Log(ctx, AuditEntry{Action: "test.auto", ActorID: "u1"})

	if !bytes.Contains(buf.Bytes(), []byte("req-123")) {
		t.Fatalf("log output = %s", buf.String())
	}
}

func TestLog_AutoTraceID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	var buf bytes.Buffer
	old := auditLogger
	auditLogger = slog.New(slog.NewJSONHandler(&buf, nil))
	defer func() { auditLogger = old }()

	tracer := otel.Tracer("audit-test")
	ctx, span := tracer.Start(context.Background(), "audit-span")
	defer span.End()

	Log(ctx, AuditEntry{Action: "test.trace", ActorID: "u1"})
	traceID := span.SpanContext().TraceID().String()
	if traceID == "" {
		t.Fatal("expected non-empty trace ID from span")
	}
	if !bytes.Contains(buf.Bytes(), []byte(traceID)) {
		t.Fatalf("log output = %s, want trace_id %q", buf.String(), traceID)
	}
}

func TestLog_AsyncChannelWrite(t *testing.T) {
	old := dbLogger
	defer func() { dbLogger = old }()

	pool := unreachableAuditPool(t)
	dbLogger = &dbAuditLogger{
		pool:   pool,
		secret: []byte("audit-secret-key-for-hmac-chain!!"),
		retry:  testRetryPolicy(),
		ch:     make(chan dbEntry, 4),
		done:   make(chan struct{}),
	}
	go dbLogger.processLoop()

	Log(context.Background(), AuditEntry{Action: "async.enqueue", ActorID: "u1"})
	close(dbLogger.ch)
	select {
	case <-dbLogger.done:
	case <-time.After(2 * time.Second):
		t.Fatal("processLoop did not finish")
	}
}

// TestDBAuditLogger_writeToDB_RetriesAndSucceeds verifies that writeToDB retries
// transient failures and eventually succeeds (v2-R-37).
func TestDBAuditLogger_writeToDB_RetriesAndSucceeds(t *testing.T) {
	pool := &flakyAuditPool{failN: 2, failErr: io.EOF}
	l := &dbAuditLogger{
		pool:   pool,
		secret: []byte("audit-secret-key-for-hmac-chain!!"),
		retry:  testRetryPolicy(),
	}
	l.writeToDB(context.Background(), AuditEntry{Action: "test.retry", ActorID: "u1"})

	calls := atomic.LoadInt32(&pool.calls)
	if calls != 3 {
		t.Fatalf("expected 3 Exec calls (2 failed + 1 success), got %d", calls)
	}
}

// TestDBAuditLogger_writeToDB_RetriesExhausted verifies that when all retries are
// exhausted, writeToDB exits cleanly without panicking (v2-R-37).
func TestDBAuditLogger_writeToDB_RetriesExhausted(t *testing.T) {
	pool := &flakyAuditPool{failN: 100, failErr: io.EOF}
	l := &dbAuditLogger{
		pool:   pool,
		secret: []byte("audit-secret-key-for-hmac-chain!!"),
		retry:  testRetryPolicy(),
	}
	l.writeToDB(context.Background(), AuditEntry{Action: "test.exhaust", ActorID: "u1"})

	// DefaultDBRetry = 3 retries → 4 total attempts
	calls := atomic.LoadInt32(&pool.calls)
	if calls != 4 {
		t.Fatalf("expected 4 Exec calls (1 initial + 3 retries), got %d", calls)
	}
}
