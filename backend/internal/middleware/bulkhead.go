package middleware

import (
	"net/http"

	"golang.org/x/sync/semaphore"
)

// Bulkhead limits concurrent requests per endpoint type using a weighted
// semaphore. When the semaphore is exhausted, additional requests are rejected
// with 503 Service Unavailable instead of queuing and exhausting shared
// downstream resources (DB connections, goroutines, Redis connections).
//
// 企业为何需要：舱壁隔离（Bulkhead）防止单类请求耗尽共享资源（DB 连接池、
// goroutine、Redis 连接池）拖垮整体服务。每类请求分配独立配额，一类耗尽
// 不影响其他类，符合故障隔离原则。TryAcquire 非阻塞：满载时快速失败而非
// 排队堆积，避免请求雪崩。
type Bulkhead struct {
	sem *semaphore.Weighted
}

// NewBulkhead creates a bulkhead with the given max concurrent requests.
func NewBulkhead(limit int64) *Bulkhead {
	return &Bulkhead{sem: semaphore.NewWeighted(limit)}
}

// Middleware returns an HTTP middleware that enforces the bulkhead.
// When the semaphore is full, returns 503 Service Unavailable with a JSON
// error body so the request never reaches downstream resources.
//
// 应作为最外层中间件使用（位于限流、认证、RBAC 之前），确保下游所有资源
// （DB 连接池、Redis 连接池、goroutine）都受到舱壁保护。
func (b *Bulkhead) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.sem.TryAcquire(1) {
			http.Error(w, `{"error":"service busy","code":"BULKHEAD_FULL"}`, http.StatusServiceUnavailable)
			return
		}
		defer b.sem.Release(1)
		next.ServeHTTP(w, r)
	})
}

// Pre-defined bulkhead quotas per request type.
// Trade-off: Lower quotas = better isolation but more 503s under load.
// These are process-global so that all routes of a given type share one pool.
var (
	// AuthBulkhead guards auth routes (login, verify, refresh, logout, check).
	AuthBulkhead = NewBulkhead(10)
	// LobbyBulkhead guards registry/lobby routes (create, list, check).
	LobbyBulkhead = NewBulkhead(10)
	// AdminBulkhead guards admin routes (config, login). Lower quota because
	// admin operations are low-volume and high-privilege.
	AdminBulkhead = NewBulkhead(3)
	// WebSocketBulkhead guards WebSocket upgrade requests. Higher quota because
	// connections are long-lived; this caps concurrent WS sessions.
	WebSocketBulkhead = NewBulkhead(50)
)
