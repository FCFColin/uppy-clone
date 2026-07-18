// Package audit provides tamper-proof audit logging.
package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sethvargo/go-retry"
	"go.opentelemetry.io/otel/trace"

	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/resilience"
)

// auditDBPool is the subset of pgxpool.Pool used by the audit logger (mockable in tests).
type auditDBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Audit logs are tamper-proof immutable records for SOC2/ISO27001 compliance.
// HMAC chain: this_hash = HMAC(secret, prev_hash || payload).

var (
	auditLogger *slog.Logger
	dbLogger    *dbAuditLogger
	dbLoggerMu  sync.RWMutex // audit-013: protects dbLogger from data race with CloseDBLogger
)

type dbAuditLogger struct {
	pool     auditDBPool
	secret   []byte
	ch       chan dbEntry
	done     chan struct{}
	mu       sync.Mutex
	lastHash string
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
// secret 用于 HMAC 链哈希。
func InitDBLogger(pool auditDBPool, secret string) {
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
		ch:     make(chan dbEntry, 1024), // buffered channel for non-blocking writes
		done:   make(chan struct{}),
	}
	// Load the last hash from DB to continue the chain
	dbLogger.loadLastHash()
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

func (l *dbAuditLogger) loadLastHash() {
	// v2-R-36: 5s timeout to prevent blocking startup when DB is unreachable.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get the most recent this_hash to continue the chain
	row := l.pool.QueryRow(ctx,
		`SELECT this_hash FROM audit_logs ORDER BY id DESC LIMIT 1`)
	var hash string
	if err := row.Scan(&hash); err == nil {
		l.lastHash = hash
	} else if err != pgx.ErrNoRows {
		// Empty table is expected on first run; other errors (incl. timeout) warrant a warning.
		slog.Warn("audit loadLastHash failed", "error", err)
	}
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
	l.mu.Lock()
	prevHash := l.lastHash

	// Compute payload for hashing
	payload, err := json.Marshal(entry)
	if err != nil {
		slog.Error("failed to marshal audit entry", "error", err, "action", entry.Action)
		l.mu.Unlock()
		return
	}
	thisHash := computeHash(l.secret, prevHash, payload)

	// v2-R-37: retry transient failures (3 retries, exponential backoff + jitter).
	// Retry happens inside the lock to preserve audit chain integrity:
	// prevHash/thisHash are captured before the loop, and lastHash is only
	// updated on success, so concurrent writers cannot break the HMAC chain.
	err = retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, execErr := l.pool.Exec(ctx,
			`INSERT INTO audit_logs (action, actor_type, actor_id, actor_ip, resource, before, after, request_id, trace_id, prev_hash, this_hash)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			entry.Action, entry.ActorType, entry.ActorID, entry.ActorIP, entry.Resource,
			entry.Before, entry.After, entry.RequestID, entry.TraceID,
			prevHash, thisHash)
		if execErr != nil {
			return resilience.MaybeRetryable(fmt.Errorf("write audit log: %w", execErr))
		}
		return nil
	})
	if err != nil {
		l.mu.Unlock()
		// audit-001: Increment metric for monitoring/alerting and write to
		// dead-letter file so compliance-critical records are not silently lost.
		metrics.AuditWriteFailures.Inc()
		slog.Error("audit writeToDB failed after retries, writing to dead-letter",
			"error", err, "action", entry.Action, "trace_id", entry.TraceID, "request_id", entry.RequestID)
		l.writeDeadLetter(entry, err)
		return
	}

	l.lastHash = thisHash
	l.mu.Unlock()
}

// writeDeadLetter writes a failed audit entry to a local JSONL file as a
// last-resort fallback (audit-001). This ensures compliance-critical audit
// records survive DB outages and can be replayed manually.
// Best-effort: if the file write also fails, the entry is lost (but the
// metric has already been incremented for alerting).
func (l *dbAuditLogger) writeDeadLetter(entry AuditEntry, writeErr error) {
	dl := map[string]interface{}{
		"entry":     entry,
		"error":     writeErr.Error(),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(dl)
	if err != nil {
		slog.Error("audit dead-letter marshal failed", "error", err)
		return
	}
	f, err := os.OpenFile("audit_deadletter.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Error("audit dead-letter file open failed", "error", err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(data, '\n')); err != nil {
		slog.Error("audit dead-letter file write failed", "error", err)
	}
}

// computeHash = HMAC-SHA256(secret, prevHash || payload)。独立函数便于单元测试。
func computeHash(secret []byte, prevHash string, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(prevHash))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
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
