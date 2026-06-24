package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Enterprise rationale: Kubernetes/Cloud Run rely on probes to determine
// service health. Without probes, the orchestrator cannot:
// - Restart a dead instance (liveness)
// - Stop sending traffic to an unready instance (readiness)
// - Wait for slow startup before health checks (startup)
// Three probe types serve distinct purposes:
// - Liveness: "Is the process alive?" → restart if not
// - Readiness: "Can it serve traffic?" → remove from LB if not
// - Startup: "Has it finished initializing?" → delay other probes

// healthCheckTimeout is the max time to wait for a dependency ping.
// 企业为何需要：readiness 探测不应阻塞过久，否则 K8s 会因 probe timeout 杀死 pod。
// 500ms 足以检测本地网络内的 PG/Redis，超过则视为不可用（熔断器可能已 open）。
const healthCheckTimeout = 500 * time.Millisecond

// Checker holds dependencies needed for health checks.
type Checker struct {
	pool        *pgxpool.Pool
	redis       *redis.Client
	canAcceptWS func() bool // optional: returns false if WS connections are at capacity
}

// NewChecker creates a health checker with the given dependencies.
func NewChecker(pool *pgxpool.Pool, rdb *redis.Client) *Checker {
	return &Checker{pool: pool, redis: rdb}
}

// WithCanAcceptWS sets an optional callback that reports whether the Hub can
// accept new WebSocket connections. When set and returning false, the readiness
// probe returns 503 so the load balancer stops sending traffic.
// 企业为何需要：舱壁隔离（Bulkhead）要求连接满时不再接收新连接，readiness 探测
// 必须反映这一状态，否则 LB 会持续向已饱和的实例转发流量。
func (c *Checker) WithCanAcceptWS(fn func() bool) *Checker {
	c.canAcceptWS = fn
	return c
}

// LiveHandler returns 200 if the process is alive (always true if handler runs).
// K8s liveness probe: restart the pod if this fails.
func (c *Checker) LiveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

// ReadyHandler checks DB and Redis connectivity with a short timeout.
// K8s readiness probe: remove pod from service endpoints if this fails.
//
// 企业为何需要：区分 degraded 与 not ready 让运维精准判断故障严重程度：
//   - PG 不可用 → not ready（关键依赖，移出 LB）
//   - Redis 不可用但 PG 可用 → degraded（仍可服务，但降级）
//   - WS 连接满 → not ready（容量饱和，移出 LB）
//
// 熔断器状态间接反映在 ping 结果中：熔断器 open 时 ping 会快速失败。
func (c *Checker) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	pgOK := true
	redisOK := true

	// Check PostgreSQL with timeout — PG is a critical dependency.
	if c.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()
		if err := c.pool.Ping(ctx); err != nil {
			checks["postgres"] = "unavailable"
			pgOK = false
		} else {
			checks["postgres"] = "ok"
		}
	}

	// Check Redis with timeout — Redis is a non-critical (degradable) dependency.
	if c.redis != nil {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()
		if err := c.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unavailable"
			redisOK = false
		} else {
			checks["redis"] = "ok"
		}
	}

	// Check WebSocket capacity — if at limit, stop receiving new connections.
	wsOK := true
	if c.canAcceptWS != nil && !c.canAcceptWS() {
		checks["websocket"] = "at capacity"
		wsOK = false
	} else {
		checks["websocket"] = "ok"
	}

	// Determine status: PG down or WS full → not ready; Redis down → degraded.
	status := "ready"
	code := http.StatusOK
	if !pgOK || !wsOK {
		status = "not ready"
		code = http.StatusServiceUnavailable
	} else if !redisOK {
		status = "degraded"
		code = http.StatusOK // still serve traffic, but signal degradation
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"checks": checks,
	})
}
