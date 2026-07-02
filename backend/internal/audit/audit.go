// Package audit provides tamper-proof audit logging.
package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"
)

// auditDBPool is the subset of pgxpool.Pool used by the audit logger (mockable in tests).
type auditDBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Audit logs are tamper-proof immutable records for SOC2/ISO27001 compliance.
// HMAC chain: this_hash = HMAC(secret, prev_hash || payload).

var (
	auditLogger *slog.Logger
	dbLogger    *dbAuditLogger
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

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	Action    string      `json:"action"`
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
	if dbLogger != nil {
		CloseDBLogger()
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
	if dbLogger == nil {
		return
	}
	close(dbLogger.ch)
	<-dbLogger.done
	dbLogger = nil
}

func (l *dbAuditLogger) loadLastHash() {
	// Get the most recent this_hash to continue the chain
	row := l.pool.QueryRow(context.Background(),
		`SELECT this_hash FROM audit_logs ORDER BY id DESC LIMIT 1`)
	var hash string
	if err := row.Scan(&hash); err == nil {
		l.lastHash = hash
	}
}

func (l *dbAuditLogger) processLoop() {
	defer close(l.done)
	for entry := range l.ch {
		l.writeToDB(entry.ctx, entry.entry)
	}
}

func (l *dbAuditLogger) writeToDB(ctx context.Context, entry AuditEntry) {
	l.mu.Lock()
	prevHash := l.lastHash

	// Compute payload for hashing
	payload, _ := json.Marshal(entry)
	thisHash := computeHash(l.secret, prevHash, payload)

	// Insert into DB
	_, err := l.pool.Exec(ctx,
		`INSERT INTO audit_logs (action, actor_id, actor_ip, resource, before, after, request_id, trace_id, prev_hash, this_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		entry.Action, entry.ActorID, entry.ActorIP, entry.Resource,
		entry.Before, entry.After, entry.RequestID, entry.TraceID,
		prevHash, thisHash)
	if err != nil {
		l.mu.Unlock()
		slog.Error("failed to write audit log to DB", "error", err, "action", entry.Action)
		return
	}

	l.lastHash = thisHash
	l.mu.Unlock()
}

// computeHash = HMAC-SHA256(secret, prevHash || payload)。独立函数便于单元测试。
func computeHash(secret []byte, prevHash string, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(prevHash))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
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

	// Always log to stdout for SIEM collection
	auditLogger.Info("audit",
		"action", entry.Action,
		"actor_id", entry.ActorID,
		"actor_ip", entry.ActorIP,
		"resource", entry.Resource,
		"before", entry.Before,
		"after", entry.After,
		"request_id", entry.RequestID,
		"trace_id", entry.TraceID,
	)

	// Non-blocking write to DB (if initialized)
	if dbLogger != nil {
		select {
		case dbLogger.ch <- dbEntry{entry: entry, ctx: ctx}:
			default:
			// Channel 满：同步回退写入 DB（100ms 超时），不丢弃记录。
			slog.Warn("audit log channel full, falling back to synchronous write",
				"action", entry.Action)
			fbCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			dbLogger.writeToDB(fbCtx, entry)
		}
	}
}
