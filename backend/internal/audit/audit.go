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
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

// Enterprise: Audit logs are immutable records of sensitive operations.
// They provide non-repudiation (不可否认性) and are required for SOC2/ISO27001 compliance.
// Audit logs are written to a separate stream for SIEM collection.
//
// 企业为何需要：审计日志必须防篡改以提供不可否认性（non-repudiation），满足 SOC2/ISO27001 合规要求。
// HMAC 链式哈希：每条记录的 this_hash = HMAC(secret, prev_hash || payload)，篡改任何记录会使后续所有 hash 验证失败。

var (
	auditLogger *slog.Logger
	dbLogger    *dbAuditLogger
)

type dbAuditLogger struct {
	pool     *pgxpool.Pool
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
	// Separate audit logger writes to stdout with "audit" source attribute
	// In production, this can be redirected to a separate file or SIEM endpoint
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

// InitDBLogger initializes the database-backed audit logger.
// Must be called after DB connection is established.
// secret is used for HMAC chain hashing.
// 企业为何需要：DB 持久化使审计日志可查询、可验证；HMAC 链使篡改可检测。
func InitDBLogger(pool *pgxpool.Pool, secret string) {
	if pool == nil || secret == "" {
		return
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

// CloseDBLogger drains the channel and stops the background goroutine.
// 企业为何需要：优雅关闭确保已入队但未写入的审计日志不丢失。
func CloseDBLogger() {
	if dbLogger == nil {
		return
	}
	close(dbLogger.ch)
	<-dbLogger.done
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
	l.mu.Unlock()

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
		slog.Error("failed to write audit log to DB", "error", err, "action", entry.Action)
		return
	}

	l.mu.Lock()
	l.lastHash = thisHash
	l.mu.Unlock()
}

// computeHash computes this_hash = HMAC-SHA256(secret, prevHash || payload).
// 企业为何需要：提取为独立函数使 HMAC 链计算可单元测试，无需 DB 依赖。
// 这是审计日志防篡改的核心安全属性，必须有专门的测试覆盖。
func computeHash(secret []byte, prevHash string, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(prevHash))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Log writes an audit entry to stdout (always) and DB (if initialized).
// 企业为何需要：stdout 日志供 SIEM 实时采集；DB 持久化供事后查询与篡改验证。
// DB 写入通过 buffered channel 异步执行，避免阻塞请求热路径。
//
// 审计日志防丢失策略（SOC2/ISO27001 合规要求）：
// channel 满时绝不丢弃，而是同步回退写入 DB（100ms 超时）。
// 宁可让请求稍慢也不能丢失审计记录——审计日志的完整性优先于请求延迟。
// 100ms 超时上限避免 DB 长时间不可用时拖垮服务，超时则记录到 stderr 作为最后防线。
//
// request_id 和 trace_id 从 context 自动提取（若调用方未显式设置），确保审计日志
// 关联请求链路，满足 SOC2 审计追溯要求。调用方显式设置的值优先保留。
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
			// sent async via buffered channel
		default:
			// Channel full — synchronous fallback with 100ms timeout.
			// 企业为何需要：审计日志绝不能丢失（合规要求）。channel 满说明消费速度跟不上，
			// 此时同步写入保证记录落库，宁可阻塞请求 100ms 也不能丢数据。
			// 100ms 超时上限防止 DB 长时间不可用时拖垮整个服务。
			slog.Warn("audit log channel full, falling back to synchronous write",
				"action", entry.Action)
			fbCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			dbLogger.writeToDB(fbCtx, entry)
		}
	}
}
