// Package metrics registers Prometheus collectors for HTTP, DB, Redis, and game SLIs.
package metrics

import (
	"log/slog"

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
	// audit-025: Log registration errors instead of silently swallowing them.
	// In tests, multiple packages may import metrics causing duplicate registration;
	// this is expected and only logged at Debug level.
	if err := prometheus.Register(collectors.NewGoCollector()); err != nil {
		slog.Debug("metrics: Go collector already registered (expected in multi-package tests)", "error", err)
	}
	if err := prometheus.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		slog.Debug("metrics: process collector already registered (expected in multi-package tests)", "error", err)
	}
}

// SLOBuckets are histogram buckets optimized for HTTP API latency.
// Enterprise rationale: Default Prometheus buckets include 10s which is too coarse
// for interactive APIs. SLO targets are typically p99 < 500ms, so we need finer
// granularity in the sub-second range and remove the 10s bucket.
var SLOBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0}

// Prometheus label names shared across multiple collectors.
const (
	labelStatus   = "status"
	labelRoomCode = "room_code"
	labelWorker   = "worker"
)

var (
	// HTTPRequestsTotal counts HTTP requests by method, path, and status.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", labelStatus},
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
	}, []string{labelRoomCode})

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
	}, []string{"endpoint", labelStatus})

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
	}, []string{labelStatus})

	// RoomCreationDuration records room creation latency in seconds.
	// audit-022: Changed from NewHistogramVec with empty labels to NewHistogram
	// to avoid unnecessary label overhead.
	RoomCreationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "room_creation_duration_seconds",
		Help:    "Room creation duration in seconds",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
	})

	// GameTickDuration records game tick processing time in seconds.
	// audit-014: Renamed from milliseconds to seconds for Prometheus naming convention consistency.
	GameTickDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "game_tick_duration_seconds",
			Help:    "Game tick processing time in seconds",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1},
		},
	)

	// WSConnectionTotal counts WebSocket connection attempts by status.
	WSConnectionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_connection_total",
		Help: "Total WebSocket connection attempts by status",
	}, []string{labelStatus})

	// WSMessageDuration records WebSocket message processing latency in seconds.
	WSMessageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ws_message_duration_seconds",
		Help:    "WebSocket message processing duration in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
	}, []string{"msg_type"})

	// GameResultsStreamLen tracks pending messages in the game.events Redis Stream.
	GameResultsStreamLen = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "game_results_stream_length",
		Help: "Number of pending messages in game.events Redis Stream",
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
	}, []string{labelRoomCode})

	// RoomPersistLagSeconds is time since the last successful persist for a room.
	RoomPersistLagSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "room_persist_lag_seconds",
		Help: "Seconds since last successful lobby state persist",
	}, []string{labelRoomCode})

	// RoomPersistDropped counts persist jobs dropped due to queue full.
	RoomPersistDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "room_persist_dropped_total",
		Help: "Total number of persist jobs dropped because the persist queue was full",
	})

	// AuditWriteFailures counts audit log DB write failures after all retries (audit-001).
	// These represent audit entries that were dead-lettered — compliance-critical data loss.
	AuditWriteFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "audit_write_failures_total",
		Help: "Total audit log write failures after retries (dead-lettered entries)",
	})

	// OutboxPublishFailures counts outbox publish failures (audit-009).
	// Redis XAdd pipeline errors mean events were not published to streams.
	OutboxPublishFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_publish_failures_total",
		Help: "Total outbox publish failures (Redis XAdd pipeline errors)",
	})

	// GameResultMarshalFailures counts game result/outbox payload marshal failures (game-019).
	// When marshalling fails, the outbox event is skipped — the Redis Stream path still
	// provides reliable delivery, but the failure should be observable.
	GameResultMarshalFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "game_result_marshal_failures_total",
		Help: "Total game result/outbox payload marshal failures (outbox event skipped)",
	})

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

	// WorkerMessagesProcessed counts worker messages by worker name and outcome (v2-R-43).
	// result label values: success, failure, deadletter, invalid_payload, skipped.
	WorkerMessagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "worker_messages_processed_total",
			Help: "Total worker messages processed by worker and result",
		},
		[]string{labelWorker, "result"},
	)

	// WorkerProcessingDuration records per-message worker processing latency in seconds (v2-R-43).
	WorkerProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "worker_processing_seconds",
			Help:    "Worker per-message processing duration in seconds",
			Buckets: SLOBuckets,
		},
		[]string{labelWorker},
	)

	// WorkerReadErrors counts transient XReadGroup errors per worker (v2-R-43).
	// Used to alert on consumer-group connectivity issues independent of message outcomes.
	WorkerReadErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "worker_read_errors_total",
			Help: "Total worker XReadGroup errors by worker",
		},
		[]string{labelWorker},
	)

	// WorkerAckErrors counts XAck failures per worker.
	// XAck failures leave messages in PEL, causing at-least-once duplicates.
	WorkerAckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "worker_ack_errors_total",
			Help: "Total worker XAck errors by worker",
		},
		[]string{labelWorker},
	)
)

// GameSessionsTotal counts game sessions started across the server.
var GameSessionsTotal = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "game_sessions_total",
		Help: "Total number of game sessions started",
	},
)
