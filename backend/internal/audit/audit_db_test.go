package audit

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestInitDBLogger_EmptySecretNoOp(t *testing.T) {
	old := dbLogger
	defer func() { dbLogger = old }()

	InitDBLogger(nil, "")
	if dbLogger != nil {
		t.Fatal("InitDBLogger with empty secret should not initialize dbLogger")
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

func TestLog_SyncFallbackWhenChannelFull(t *testing.T) {
	old := dbLogger
	defer func() { dbLogger = old }()

	pool := unreachableAuditPool(t)

	dbLogger = &dbAuditLogger{
		pool:   pool,
		secret: []byte("audit-secret-key-for-hmac-chain!!"),
		ch:     make(chan dbEntry, 1),
		done:   make(chan struct{}),
	}
	go dbLogger.processLoop()

	dbLogger.ch <- dbEntry{entry: AuditEntry{Action: "block"}, ctx: context.Background()}
	Log(context.Background(), AuditEntry{Action: "sync.fallback", ActorID: "u1"})

	close(dbLogger.ch)
	select {
	case <-dbLogger.done:
	case <-time.After(2 * time.Second):
		t.Fatal("processLoop did not finish")
	}
}

func TestCloseDBLogger_DrainsQueue(t *testing.T) {
	old := dbLogger
	defer func() { dbLogger = old }()

	pool := unreachableAuditPool(t)
	InitDBLogger(pool, "audit-secret-key-for-hmac-chain!!")
	Log(context.Background(), AuditEntry{Action: "queued", ActorID: "u1"})
	CloseDBLogger()
}

func TestDBAuditLogger_loadLastHash_QueryError(t *testing.T) {
	pool := unreachableAuditPool(t)
	l := &dbAuditLogger{pool: pool, secret: []byte("audit-secret-key-for-hmac-chain!!")}
	l.loadLastHash()
	if l.lastHash != "" {
		t.Fatalf("expected empty lastHash on query error, got %q", l.lastHash)
	}
}

func TestDBAuditLogger_writeToDB_ExecError(t *testing.T) {
	pool := unreachableAuditPool(t)
	l := &dbAuditLogger{
		pool:     pool,
		secret:   []byte("audit-secret-key-for-hmac-chain!!"),
		lastHash: "prev-hash",
	}
	l.writeToDB(context.Background(), AuditEntry{Action: "test.write", ActorID: "u1"})
	if l.lastHash != "prev-hash" {
		t.Fatalf("lastHash should remain unchanged on exec error, got %q", l.lastHash)
	}
}

func TestLog_AsyncChannelWrite(t *testing.T) {
	old := dbLogger
	defer func() { dbLogger = old }()

	pool := unreachableAuditPool(t)
	dbLogger = &dbAuditLogger{
		pool:   pool,
		secret: []byte("audit-secret-key-for-hmac-chain!!"),
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
