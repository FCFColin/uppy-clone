// Package metrics registers Prometheus collectors for HTTP, DB, Redis, and game SLIs.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
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
	if err := prometheus.Register(collectors.NewGoCollector()); err != nil {
		_ = err // already registered in tests with multiple packages
	}
	if err := prometheus.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		_ = err // already registered
	}
}

// SLOBuckets are histogram buckets optimized for HTTP API latency.
// Enterprise rationale: Default Prometheus buckets include 10s which is too coarse
// for interactive APIs. SLO targets are typically p99 < 500ms, so we need finer
// granularity in the sub-second range and remove the 10s bucket.
var SLOBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0}

var (
	// HTTPRequestsTotal counts HTTP requests by method, path, and status.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	// HTTPRequestDuration records HTTP request latency in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: SLOBuckets,
		},
		[]string{"method", "path"},
	)

	// DBPoolAcquireCount counts connection acquires from the PostgreSQL pool.
	DBPoolAcquireCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "db_pool_acquire_total",
			Help: "Total number of connection acquires from the pool",
		},
	)
	// DBPoolAcquireDuration records time spent waiting to acquire a DB pool connection.
	DBPoolAcquireDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "db_pool_acquire_duration_seconds",
			Help:    "Time waited to acquire a connection from the pool",
			Buckets: SLOBuckets,
		},
	)
	// DBPoolIdleConns tracks idle connections in the PostgreSQL pool.
	DBPoolIdleConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_pool_idle_conns",
			Help: "Number of idle connections in the pool",
		},
	)
	// DBPoolInUseConns tracks in-use connections in the PostgreSQL pool.
	DBPoolInUseConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_pool_in_use_conns",
			Help: "Number of connections currently in use",
		},
	)

	// ActiveRooms tracks the number of active game rooms.
	ActiveRooms = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_active_rooms",
			Help: "Number of active game rooms",
		},
	)
	// ActivePlayers tracks total connected players across all rooms.
	ActivePlayers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_active_players",
			Help: "Total number of players across all rooms",
		},
	)
	// WSConnections tracks active WebSocket connections across the server.
	WSConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_connections",
			Help: "Number of active WebSocket connections",
		},
	)

	// WSMessagesDroppedTotal counts messages dropped due to slow WebSocket clients.
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

	// RedisPoolIdleConns tracks idle connections in the Redis pool.
	RedisPoolIdleConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_pool_idle_conns",
			Help: "Number of idle connections in the Redis pool",
		},
	)
	// RedisPoolTotalConns tracks total connections in the Redis pool.
	RedisPoolTotalConns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_pool_total_conns",
			Help: "Total number of connections in the Redis pool",
		},
	)

	// AdminLoginLockedTotal counts admin login attempts rejected by IP lockout.
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

	// AuthRequestTotal counts auth endpoint requests by endpoint and status.
	AuthRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auth_requests_total",
		Help: "Total auth requests by endpoint and status",
	}, []string{"endpoint", "status"})

	// AuthRequestDuration records auth endpoint latency in seconds.
	AuthRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "auth_request_duration_seconds",
		Help:    "Auth request duration in seconds",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
	}, []string{"endpoint"})

	// RoomCreationTotal counts room creation attempts by status.
	RoomCreationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "room_creation_total",
		Help: "Total room creation attempts by status",
	}, []string{"status"})

	// RoomCreationDuration records room creation latency in seconds.
	RoomCreationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "room_creation_duration_seconds",
		Help:    "Room creation duration in seconds",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
	}, []string{})

	// WSConnectionTotal counts WebSocket connection attempts by status.
	WSConnectionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_connection_total",
		Help: "Total WebSocket connection attempts by status",
	}, []string{"status"})

	// WSMessageDuration records WebSocket message processing latency in seconds.
	WSMessageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ws_message_duration_seconds",
		Help:    "WebSocket message processing duration in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
	}, []string{"msg_type"})

	// GameResultsStreamLen tracks pending messages in the game:results Redis Stream.
	GameResultsStreamLen = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "game_results_stream_length",
		Help: "Number of pending messages in game:results Redis Stream",
	})
	// EmailQueueStreamLen tracks pending messages in the email queue Redis Stream.
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

	// RoomLockHoldSeconds measures time spent holding Room.mu by operation type.
	RoomLockHoldSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "room_lock_hold_seconds",
		Help:    "Duration Room.mu was held by operation",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"operation"})

	// RoomOutboundQueueDepth is the number of pending outbound broadcast messages per room.
	RoomOutboundQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "room_outbound_queue_depth",
		Help: "Pending outbound broadcast messages awaiting delivery",
	}, []string{"room_code"})

	// RoomPersistLagSeconds is time since the last successful persist for a room.
	RoomPersistLagSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "room_persist_lag_seconds",
		Help: "Seconds since last successful lobby state persist",
	}, []string{"room_code"})

	// OutboxLagSeconds is the age of the oldest unprocessed outbox event.
	OutboxLagSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "outbox_lag_seconds",
		Help: "Age in seconds of the oldest unprocessed outbox event",
	})

	// OutboxBatchSize records the number of events processed per outbox poll cycle.
	OutboxBatchSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "outbox_batch_size",
		Help:    "Number of outbox events processed per batch",
		Buckets: []float64{1, 5, 10, 25, 50, 100},
	})
)

// GameSessionsTotal counts game sessions started across the server.
var GameSessionsTotal = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "game_sessions_total",
		Help: "Total number of game sessions started",
	},
)
