// Package audit provides audit logging.
package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sethvargo/go-retry"
	"go.opentelemetry.io/otel/trace"

	"github.com/uppy-clone/backend/internal/metrics"
)

// auditDBPool is the subset of pgxpool.Pool used by the audit logger (mockable in tests).
type auditDBPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// RetryPolicy holds the retry backoff and error classifier for audit DB writes.
type RetryPolicy struct {
	DBRetry        retry.Backoff
	MaybeRetryable func(error) error
}

var (
	auditLogger *slog.Logger
	dbLogger    *dbAuditLogger
	dbLoggerMu  sync.RWMutex // audit-013: protects dbLogger from data race with CloseDBLogger
)

type dbAuditLogger struct {
	pool   auditDBPool
	secret []byte
	retry  RetryPolicy
	ch     chan dbEntry
	done   chan struct{}
}

type dbEntry struct {
	entry AuditEntry
	ctx   context.Context
}

func init() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	auditLogger = slog.New(handler.WithGroup("audit"))
}

// ActorType constants define the semantic category of an audit actor.
// project-08-003: Previously ActorID was overloaded with mixed semantics
// (UUID for users, "admin" for admins, "system" for automated actions,
// role strings for RBAC denials). ActorType disambiguates these so that
// SIEM consumers and compliance queries can reliably filter by actor kind.
const (
	ActorTypeSystem    = "system"    // automated/background process
	ActorTypeUser      = "user"      // authenticated end-user (ActorID = user UUID)
	ActorTypeAdmin     = "admin"     // administrative operator (ActorID = "admin")
	ActorTypeAnonymous = "anonymous" // unauthenticated request (RBAC deny, etc.)
)

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	Action    string      `json:"action"`
	ActorType string      `json:"actor_type,omitempty"`
	ActorID   string      `json:"actor_id"`
	ActorIP   string      `json:"actor_ip"`
	Resource  string      `json:"resource"`
	Before    interface{} `json:"before,omitempty"`
	After     interface{} `json:"after,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	TraceID   string      `json:"trace_id,omitempty"`
}

// InitDBLogger 初始化数据库审计日志。必须在 DB 连接建立后调用。
// secret 用于 HMAC-SHA256 哈希 actor_ip（隐私合规）。
func InitDBLogger(pool auditDBPool, secret string, retryPolicy RetryPolicy) {
	if pool == nil || secret == "" {
		return
	}
	dbLoggerMu.Lock()
	defer dbLoggerMu.Unlock()
	if dbLogger != nil {
		closeDBLoggerLocked()
	}
	dbLogger = &dbAuditLogger{
		pool:   pool,
		secret: []byte(secret),
		retry:  retryPolicy,
		ch:     make(chan dbEntry, 1024),
		done:   make(chan struct{}),
	}
	go dbLogger.processLoop()
}

// CloseDBLogger 排空 channel 并停止后台协程，确保已入队的审计日志不丢失。
func CloseDBLogger() {
	dbLoggerMu.Lock()
	defer dbLoggerMu.Unlock()
	closeDBLoggerLocked()
}

// closeDBLoggerLocked performs the actual close without acquiring the lock.
// Caller must hold dbLoggerMu.
func closeDBLoggerLocked() {
	if dbLogger == nil {
		return
	}
	close(dbLogger.ch)
	<-dbLogger.done
	dbLogger = nil
}

func (l *dbAuditLogger) processLoop() {
	defer close(l.done)
	for entry := range l.ch {
		// audit-005: Use detached context instead of the original request context.
		// The request context may be canceled after the HTTP response is sent,
		// which would cause retry.Do to fail immediately with context.Canceled.
		// A fresh context with a generous timeout ensures audit writes complete
		// even after the request lifecycle ends.
		writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		l.writeToDB(writeCtx, entry.entry)
		cancel()
	}
}

func (l *dbAuditLogger) writeToDB(ctx context.Context, entry AuditEntry) {
	// v2-R-37: retry transient failures (3 retries, exponential backoff + jitter).
	err := retry.Do(ctx, l.retry.DBRetry, func(ctx context.Context) error {
		_, execErr := l.pool.Exec(ctx,
			`INSERT INTO audit_logs (action, actor_type, actor_id, actor_ip, resource, before, after, request_id, trace_id, prev_hash, this_hash)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			entry.Action, entry.ActorType, entry.ActorID, entry.ActorIP, entry.Resource,
			entry.Before, entry.After, entry.RequestID, entry.TraceID,
			"", "")
		if execErr != nil {
			return l.retry.MaybeRetryable(fmt.Errorf("write audit log: %w", execErr))
		}
		return nil
	})
	if err != nil {
		// audit-001: Increment metric for monitoring/alerting.
		metrics.AuditWriteFailures.Inc()
		slog.Error("audit writeToDB failed after retries",
			"error", err, "action", entry.Action, "trace_id", entry.TraceID, "request_id", entry.RequestID)
	}
}

// hashActorIP computes HMAC-SHA256 of the actor IP before storage.
// Uses the audit secret as HMAC key to prevent rainbow-table reversal of IPv4.
// Falls back to unkeyed SHA-256 only when no DB logger is initialized (e.g. tests).
func hashActorIP(ip string) string {
	if ip == "" {
		return ""
	}
	dbLoggerMu.RLock()
	l := dbLogger
	dbLoggerMu.RUnlock()
	if l != nil {
		mac := hmac.New(sha256.New, l.secret)
		mac.Write([]byte(ip))
		return hex.EncodeToString(mac.Sum(nil))
	}
	h := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(h[:])
}

// Log 写入审计条目到 stdout（始终）和 DB（若已初始化）。
// DB 写入通过 buffered channel 异步执行；channel 满时同步回退（100ms 超时），不丢弃记录。
func Log(ctx context.Context, entry AuditEntry) {
	// Auto-populate request_id and trace_id from context if not already set,
	// so audit entries always carry request correlation metadata.
	if entry.RequestID == "" {
		entry.RequestID = middleware.GetReqID(ctx)
	}
	if entry.TraceID == "" {
		if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
			entry.TraceID = span.SpanContext().TraceID().String()
		}
	}

	// Hash actor IP with HMAC-SHA256 before storage for privacy compliance
	entry.ActorIP = hashActorIP(entry.ActorIP)

	// Always log to stdout for SIEM collection
	auditLogger.Info("audit",
		"action", entry.Action,
		"actor_type", entry.ActorType,
		"actor_id", entry.ActorID,
		"actor_ip", entry.ActorIP,
		"resource", entry.Resource,
		"before", entry.Before,
		"after", entry.After,
		"request_id", entry.RequestID,
		"trace_id", entry.TraceID,
	)

	// Non-blocking write to DB (if initialized)
	// audit-013: Use RLock to safely access dbLogger without data race.
	dbLoggerMu.RLock()
	l := dbLogger
	dbLoggerMu.RUnlock()
	if l != nil {
		select {
		case l.ch <- dbEntry{entry: entry, ctx: ctx}:
		default:
			// Channel 满：同步回退写入 DB（100ms 超时），不丢弃记录。
			// audit-005: Use context.Background() instead of request ctx — the
			// request context may be canceled after the HTTP response is sent,
			// which would cause the fallback write to fail immediately.
			slog.Warn("audit log channel full, falling back to synchronous write",
				"action", entry.Action)
			fbCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			l.writeToDB(fbCtx, entry)
		}
	}
}
