package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Enterprise rationale: Without metrics, production operations are blind.
// Prometheus is the CNCF-graduated standard for Kubernetes monitoring.
// Golden Signals (Latency, Traffic, Errors, Saturation) require these metrics.
// Trade-off: /metrics endpoint exposes internal state; restrict access via
// network policy in production (only Prometheus scraper can reach it).

func init() {
	// Register Go runtime and process collectors for infrastructure observability.
	// Enterprise rationale: Go runtime metrics (goroutine count, GC pauses, heap alloc)
	// and process metrics (RSS, FD count, CPU) are essential for capacity planning
	// and detecting memory leaks / goroutine leaks before they cause outages.
	if err := prometheus.Register(prometheus.NewGoCollector()); err != nil {
		// Already registered (e.g., in tests with multiple packages)
	}
	if err := prometheus.Register(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{})); err != nil {
		// Already registered
	}
}

// SLOBuckets are histogram buckets optimized for HTTP API latency.
// Enterprise rationale: Default Prometheus buckets include 10s which is too coarse
// for interactive APIs. SLO targets are typically p99 < 500ms, so we need finer
// granularity in the sub-second range and remove the 10s bucket.
var SLOBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0}

var (
	// HTTP metrics — Golden Signal: Latency + Traffic + Errors
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: SLOBuckets,
		},
		[]string{"method", "path"},
	)

	// DB pool metrics — Golden Signal: Saturation
	DBPoolAcquireCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "db_pool_acquire_total",
			Help: "Total number of connection acquires from the pool",
		},
	)
	DBPoolAcquireDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "db_pool_acquire_duration_seconds",
			Help:    "Time waited to acquire a connection from the pool",
			Buckets: SLOBuckets,
		},
	)
	DBPoolIdleConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_pool_idle_conns",
			Help: "Number of idle connections in the pool",
		},
	)
	DBPoolInUseConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_pool_in_use_conns",
			Help: "Number of connections currently in use",
		},
	)

	// Business metrics
	ActiveRooms = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_active_rooms",
			Help: "Number of active game rooms",
		},
	)
	ActivePlayers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_active_players",
			Help: "Total number of players across all rooms",
		},
	)
	WSConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_connections",
			Help: "Number of active WebSocket connections",
		},
	)
	GameSessionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "game_sessions_total",
			Help: "Total number of game sessions started",
		},
	)

	// WS message drop metric — slow client detection.
	// 企业为何需要：广播消息被静默丢弃时运维无法感知。丢弃率突增表明客户端消费速度
	// 跟不上生产速度，需告警排查慢客户端或扩容 WebSocket 层。
	WSMessagesDroppedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_messages_dropped_total",
		Help: "Total WebSocket messages dropped due to full channel buffer",
	}, []string{"room_code"})

	// CircuitBreakerState tracks the state of each circuit breaker.
	// 企业为何需要：熔断器状态变更必须可观测。否则运维无法知道下游依赖是否被熔断，也无法设置告警。
	// Values: 0=closed (healthy), 0.5=half-open (probing), 1=open (tripped).
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Current state of circuit breakers: 0=closed, 0.5=half-open, 1=open",
		},
		[]string{"name", "state"},
	)

	// Redis pool metrics — Golden Signal: Saturation
	RedisPoolIdleConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_pool_idle_conns",
			Help: "Number of idle connections in the Redis pool",
		},
	)
	RedisPoolTotalConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_pool_total_conns",
			Help: "Total number of connections in the Redis pool",
		},
	)

	// Admin login lockout metric — brute-force defense observability.
	// 企业为何需要：登录锁定次数突增表明暴力破解攻击正在进行，需告警。
	AdminLoginLockedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "admin_login_locked_total",
		Help: "Total number of admin login attempts rejected due to lockout",
	})

	// SuspiciousLoginTotal tracks anomalous login behavior (multi-IP, brute force).
	// 企业为何需要：多 IP 同账户登录是账户盗用的典型信号，需告警并纳入安全监控。
	SuspiciousLoginTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "suspicious_login_total",
		Help: "Total suspicious login events (multi-IP, brute force, etc.)",
	})

	// SLO metrics — 企业为何需要：SLI 指标是 SLO 监控的基础，用于计算 Error Budget 和 Burn Rate。
	// 详见 docs/operations/slo.md。这些指标配合 deploy/alertmanager/rules.yml 的多窗口告警实现 SLO 自动化。
	AuthRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auth_requests_total",
		Help: "Total auth requests by endpoint and status",
	}, []string{"endpoint", "status"})

	AuthRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "auth_request_duration_seconds",
		Help:    "Auth request duration in seconds",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
	}, []string{"endpoint"})

	RoomCreationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "room_creation_total",
		Help: "Total room creation attempts by status",
	}, []string{"status"})

	RoomCreationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "room_creation_duration_seconds",
		Help:    "Room creation duration in seconds",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
	}, []string{})

	WSConnectionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_connection_total",
		Help: "Total WebSocket connection attempts by status",
	}, []string{"status"})

	WSMessageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ws_message_duration_seconds",
		Help:    "WebSocket message processing duration in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
	}, []string{"msg_type"})

	// Async queue metrics — Consumer Lag monitoring.
	// 企业为何需要：Stream 长度增长表明 Worker 消费速度跟不上生产速度，需告警扩容。
	GameResultsStreamLen = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "game_results_stream_length",
		Help: "Number of pending messages in game:results Redis Stream",
	})
	EmailQueueStreamLen = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "email_queue_stream_length",
		Help: "Number of pending messages in email:queue Redis Stream",
	})

	// NicknameConfirmTotal tracks SET_NICKNAME outcomes (accepted vs silently rejected).
	NicknameConfirmTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nickname_confirm_total",
		Help: "Total SET_NICKNAME messages by outcome",
	}, []string{"result"})

	// RoomsByPhase tracks how many rooms are in each game phase (polled from Hub).
	RoomsByPhase = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rooms_by_phase",
		Help: "Number of active rooms grouped by game phase",
	}, []string{"phase"})
)
