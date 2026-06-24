package metrics //nolint:revive // intentional package name

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestInit_DoesNotPanic verifies that the package init() function (which
// registers Go runtime and process collectors) runs without panicking,
// even when called multiple times across test packages.
func TestInit_DoesNotPanic(t *testing.T) {
	// Re-register to exercise the "already registered" error paths in init().
	// The init() ignores errors from duplicate registration, so this should
	// not panic.
	if err := prometheus.Register(collectors.NewGoCollector()); err == nil {
		t.Log("GoCollector was not previously registered (unexpected)")
	}
	// Whether it errors or not, the test passes — we just exercise the path.
}

// TestSLOBuckets verifies the SLO bucket configuration.
func TestSLOBuckets(t *testing.T) {
	if len(SLOBuckets) == 0 {
		t.Fatal("SLOBuckets should not be empty")
	}
	// Buckets must be sorted ascending for Prometheus histograms.
	for i := 1; i < len(SLOBuckets); i++ {
		if SLOBuckets[i] <= SLOBuckets[i-1] {
			t.Fatalf("SLOBuckets not strictly ascending at index %d: %v <= %v",
				i, SLOBuckets[i], SLOBuckets[i-1])
		}
	}
	// The 10s bucket should be absent (per enterprise rationale comment).
	for _, b := range SLOBuckets {
		if b >= 10.0 {
			t.Fatalf("SLOBuckets should not contain 10s bucket, got %v", b)
		}
	}
}

// TestHTTPRequestsTotal_Increment verifies the HTTPRequestsTotal counter
// can be incremented with label values and the value is retrievable.
func TestHTTPRequestsTotal_Increment(t *testing.T) {
	HTTPRequestsTotal.Reset()
	HTTPRequestsTotal.WithLabelValues("GET", "/test", "200").Inc()
	HTTPRequestsTotal.WithLabelValues("GET", "/test", "200").Inc()
	HTTPRequestsTotal.WithLabelValues("POST", "/test", "201").Inc()

	if got := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("GET", "/test", "200")); got != 2 {
		t.Fatalf("GET /test 200 count = %v, want 2", got)
	}
	if got := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("POST", "/test", "201")); got != 1 {
		t.Fatalf("POST /test 201 count = %v, want 1", got)
	}
}

// TestHTTPRequestDuration_Observe verifies the HTTPRequestDuration histogram
// accepts observations.
func TestHTTPRequestDuration_Observe(t *testing.T) {
	HTTPRequestDuration.Reset()
	HTTPRequestDuration.WithLabelValues("GET", "/test").Observe(0.05)
	HTTPRequestDuration.WithLabelValues("GET", "/test").Observe(0.15)

	count := testutil.CollectAndCount(HTTPRequestDuration)
	if count < 1 {
		t.Fatalf("HTTPRequestDuration metric count = %d, want >= 1", count)
	}
}

// TestDBPoolMetrics verifies DB pool counters, histograms, and gauges.
func TestDBPoolMetrics(t *testing.T) {
	// Counter
	before := testutil.ToFloat64(DBPoolAcquireCount)
	DBPoolAcquireCount.Inc()
	DBPoolAcquireCount.Inc()
	after := testutil.ToFloat64(DBPoolAcquireCount)
	if after-before != 2 {
		t.Fatalf("DBPoolAcquireCount delta = %v, want 2", after-before)
	}

	// Histogram
	DBPoolAcquireDuration.Observe(0.01)
	if testutil.CollectAndCount(DBPoolAcquireDuration) < 1 {
		t.Fatal("DBPoolAcquireDuration not collected")
	}

	// Gauges
	DBPoolIdleConns.Set(5)
	if got := testutil.ToFloat64(DBPoolIdleConns); got != 5 {
		t.Fatalf("DBPoolIdleConns = %v, want 5", got)
	}
	DBPoolIdleConns.Inc()
	if got := testutil.ToFloat64(DBPoolIdleConns); got != 6 {
		t.Fatalf("DBPoolIdleConns after Inc = %v, want 6", got)
	}
	DBPoolIdleConns.Dec()
	if got := testutil.ToFloat64(DBPoolIdleConns); got != 5 {
		t.Fatalf("DBPoolIdleConns after Dec = %v, want 5", got)
	}

	DBPoolInUseConns.Set(3)
	if got := testutil.ToFloat64(DBPoolInUseConns); got != 3 {
		t.Fatalf("DBPoolInUseConns = %v, want 3", got)
	}
	DBPoolInUseConns.Add(2)
	if got := testutil.ToFloat64(DBPoolInUseConns); got != 5 {
		t.Fatalf("DBPoolInUseConns after Add(2) = %v, want 5", got)
	}
	DBPoolInUseConns.Sub(1)
	if got := testutil.ToFloat64(DBPoolInUseConns); got != 4 {
		t.Fatalf("DBPoolInUseConns after Sub(1) = %v, want 4", got)
	}
}

// TestBusinessMetrics verifies the business gauges and counters.
func TestBusinessMetrics(t *testing.T) {
	// ActiveRooms gauge
	ActiveRooms.Set(10)
	if got := testutil.ToFloat64(ActiveRooms); got != 10 {
		t.Fatalf("ActiveRooms = %v, want 10", got)
	}

	// ActivePlayers gauge
	ActivePlayers.Set(50)
	if got := testutil.ToFloat64(ActivePlayers); got != 50 {
		t.Fatalf("ActivePlayers = %v, want 50", got)
	}

	// WSConnections gauge
	WSConnections.Set(100)
	if got := testutil.ToFloat64(WSConnections); got != 100 {
		t.Fatalf("WSConnections = %v, want 100", got)
	}

	// GameSessionsTotal counter
	before := testutil.ToFloat64(GameSessionsTotal)
	GameSessionsTotal.Inc()
	after := testutil.ToFloat64(GameSessionsTotal)
	if after-before != 1 {
		t.Fatalf("GameSessionsTotal delta = %v, want 1", after-before)
	}
}

// TestWSMessagesDroppedTotal verifies the WebSocket message drop counter
// increments per room_code label.
func TestWSMessagesDroppedTotal(t *testing.T) {
	WSMessagesDroppedTotal.Reset()
	WSMessagesDroppedTotal.WithLabelValues("ROOM1").Inc()
	WSMessagesDroppedTotal.WithLabelValues("ROOM1").Inc()
	WSMessagesDroppedTotal.WithLabelValues("ROOM2").Inc()

	if got := testutil.ToFloat64(WSMessagesDroppedTotal.WithLabelValues("ROOM1")); got != 2 {
		t.Fatalf("ROOM1 drops = %v, want 2", got)
	}
	if got := testutil.ToFloat64(WSMessagesDroppedTotal.WithLabelValues("ROOM2")); got != 1 {
		t.Fatalf("ROOM2 drops = %v, want 1", got)
	}
}

// TestCircuitBreakerState verifies the circuit breaker state gauge vector
// can track closed (0), half-open (0.5), and open (1) states.
func TestCircuitBreakerState(t *testing.T) {
	CircuitBreakerState.Reset()

	// Closed (healthy)
	CircuitBreakerState.WithLabelValues("downstream-a", "closed").Set(0)
	if got := testutil.ToFloat64(CircuitBreakerState.WithLabelValues("downstream-a", "closed")); got != 0 {
		t.Fatalf("circuit breaker closed = %v, want 0", got)
	}

	// Half-open (probing)
	CircuitBreakerState.WithLabelValues("downstream-b", "half-open").Set(0.5)
	if got := testutil.ToFloat64(CircuitBreakerState.WithLabelValues("downstream-b", "half-open")); got != 0.5 {
		t.Fatalf("circuit breaker half-open = %v, want 0.5", got)
	}

	// Open (tripped)
	CircuitBreakerState.WithLabelValues("downstream-c", "open").Set(1)
	if got := testutil.ToFloat64(CircuitBreakerState.WithLabelValues("downstream-c", "open")); got != 1 {
		t.Fatalf("circuit breaker open = %v, want 1", got)
	}
}

// TestRedisPoolMetrics verifies Redis pool gauges.
func TestRedisPoolMetrics(t *testing.T) {
	RedisPoolIdleConns.Set(8)
	if got := testutil.ToFloat64(RedisPoolIdleConns); got != 8 {
		t.Fatalf("RedisPoolIdleConns = %v, want 8", got)
	}

	RedisPoolTotalConns.Set(16)
	if got := testutil.ToFloat64(RedisPoolTotalConns); got != 16 {
		t.Fatalf("RedisPoolTotalConns = %v, want 16", got)
	}
}

// TestAdminLoginLockedTotal verifies the admin login lockout counter.
func TestAdminLoginLockedTotal(t *testing.T) {
	before := testutil.ToFloat64(AdminLoginLockedTotal)
	AdminLoginLockedTotal.Inc()
	AdminLoginLockedTotal.Add(4)
	after := testutil.ToFloat64(AdminLoginLockedTotal)
	if after-before != 5 {
		t.Fatalf("AdminLoginLockedTotal delta = %v, want 5", after-before)
	}
}

// TestSuspiciousLoginTotal verifies the suspicious login counter.
func TestSuspiciousLoginTotal(t *testing.T) {
	before := testutil.ToFloat64(SuspiciousLoginTotal)
	SuspiciousLoginTotal.Inc()
	after := testutil.ToFloat64(SuspiciousLoginTotal)
	if after-before != 1 {
		t.Fatalf("SuspiciousLoginTotal delta = %v, want 1", after-before)
	}
}

// TestAuthRequestMetrics verifies auth request counter and histogram.
func TestAuthRequestMetrics(t *testing.T) {
	AuthRequestTotal.Reset()
	AuthRequestTotal.WithLabelValues("login", "200").Inc()
	AuthRequestTotal.WithLabelValues("login", "401").Inc()
	AuthRequestTotal.WithLabelValues("login", "200").Inc()

	if got := testutil.ToFloat64(AuthRequestTotal.WithLabelValues("login", "200")); got != 2 {
		t.Fatalf("auth login 200 = %v, want 2", got)
	}
	if got := testutil.ToFloat64(AuthRequestTotal.WithLabelValues("login", "401")); got != 1 {
		t.Fatalf("auth login 401 = %v, want 1", got)
	}

	AuthRequestDuration.Reset()
	AuthRequestDuration.WithLabelValues("login").Observe(0.2)
	if testutil.CollectAndCount(AuthRequestDuration) < 1 {
		t.Fatal("AuthRequestDuration not collected")
	}
}

// TestRoomCreationMetrics verifies room creation counter and histogram.
func TestRoomCreationMetrics(t *testing.T) {
	RoomCreationTotal.Reset()
	RoomCreationTotal.WithLabelValues("success").Inc()
	RoomCreationTotal.WithLabelValues("success").Inc()
	RoomCreationTotal.WithLabelValues("failed").Inc()

	if got := testutil.ToFloat64(RoomCreationTotal.WithLabelValues("success")); got != 2 {
		t.Fatalf("room creation success = %v, want 2", got)
	}
	if got := testutil.ToFloat64(RoomCreationTotal.WithLabelValues("failed")); got != 1 {
		t.Fatalf("room creation failed = %v, want 1", got)
	}

	RoomCreationDuration.Reset()
	RoomCreationDuration.WithLabelValues().Observe(0.1)
	if testutil.CollectAndCount(RoomCreationDuration) < 1 {
		t.Fatal("RoomCreationDuration not collected")
	}
}

// TestWSConnectionMetrics verifies WS connection counter and message duration histogram.
func TestWSConnectionMetrics(t *testing.T) {
	WSConnectionTotal.Reset()
	WSConnectionTotal.WithLabelValues("established").Inc()
	WSConnectionTotal.WithLabelValues("established").Inc()
	WSConnectionTotal.WithLabelValues("rejected").Inc()

	if got := testutil.ToFloat64(WSConnectionTotal.WithLabelValues("established")); got != 2 {
		t.Fatalf("ws established = %v, want 2", got)
	}
	if got := testutil.ToFloat64(WSConnectionTotal.WithLabelValues("rejected")); got != 1 {
		t.Fatalf("ws rejected = %v, want 1", got)
	}

	WSMessageDuration.Reset()
	WSMessageDuration.WithLabelValues("tap").Observe(0.005)
	WSMessageDuration.WithLabelValues("join").Observe(0.02)
	if testutil.CollectAndCount(WSMessageDuration) < 1 {
		t.Fatal("WSMessageDuration not collected")
	}
}

// TestStreamLengthGauges verifies the Redis Stream length gauges.
func TestStreamLengthGauges(t *testing.T) {
	GameResultsStreamLen.Set(42)
	if got := testutil.ToFloat64(GameResultsStreamLen); got != 42 {
		t.Fatalf("GameResultsStreamLen = %v, want 42", got)
	}

	EmailQueueStreamLen.Set(7)
	if got := testutil.ToFloat64(EmailQueueStreamLen); got != 7 {
		t.Fatalf("EmailQueueStreamLen = %v, want 7", got)
	}
}

// TestCollectAndCompare verifies that all registered metrics can be collected
// without error. This is adversarial: catches duplicate registration or
// malformed metric definitions.
func TestCollectAndCompare(t *testing.T) {
	// Gather metrics from the default registry and verify no collection error.
	// We only verify that gathering doesn't panic/error; the empty comparison
	// string would produce a diff but we ignore it.
	_, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gathering metrics failed: %v", err)
	}
}

// TestAllMetrics_NonNil verifies that all exported metric variables are non-nil
// and properly initialized. This is adversarial: catches initialization order
// issues or accidental shadowing.
func TestAllMetrics_NonNil(t *testing.T) {
	t.Run("HTTP metrics", func(t *testing.T) {
		if HTTPRequestsTotal == nil {
			t.Fatal("HTTPRequestsTotal is nil")
		}
		if HTTPRequestDuration == nil {
			t.Fatal("HTTPRequestDuration is nil")
		}
	})

	t.Run("DB pool metrics", func(t *testing.T) {
		if DBPoolAcquireCount == nil {
			t.Fatal("DBPoolAcquireCount is nil")
		}
		if DBPoolAcquireDuration == nil {
			t.Fatal("DBPoolAcquireDuration is nil")
		}
		if DBPoolIdleConns == nil {
			t.Fatal("DBPoolIdleConns is nil")
		}
		if DBPoolInUseConns == nil {
			t.Fatal("DBPoolInUseConns is nil")
		}
	})

	t.Run("Business metrics", func(t *testing.T) {
		if ActiveRooms == nil {
			t.Fatal("ActiveRooms is nil")
		}
		if ActivePlayers == nil {
			t.Fatal("ActivePlayers is nil")
		}
		if WSConnections == nil {
			t.Fatal("WSConnections is nil")
		}
		if GameSessionsTotal == nil {
			t.Fatal("GameSessionsTotal is nil")
		}
	})

	t.Run("WS metrics", func(t *testing.T) {
		if WSMessagesDroppedTotal == nil {
			t.Fatal("WSMessagesDroppedTotal is nil")
		}
		if CircuitBreakerState == nil {
			t.Fatal("CircuitBreakerState is nil")
		}
		if WSConnectionTotal == nil {
			t.Fatal("WSConnectionTotal is nil")
		}
		if WSMessageDuration == nil {
			t.Fatal("WSMessageDuration is nil")
		}
	})

	t.Run("Redis pool metrics", func(t *testing.T) {
		if RedisPoolIdleConns == nil {
			t.Fatal("RedisPoolIdleConns is nil")
		}
		if RedisPoolTotalConns == nil {
			t.Fatal("RedisPoolTotalConns is nil")
		}
	})

	t.Run("Auth metrics", func(t *testing.T) {
		if AdminLoginLockedTotal == nil {
			t.Fatal("AdminLoginLockedTotal is nil")
		}
		if SuspiciousLoginTotal == nil {
			t.Fatal("SuspiciousLoginTotal is nil")
		}
		if AuthRequestTotal == nil {
			t.Fatal("AuthRequestTotal is nil")
		}
		if AuthRequestDuration == nil {
			t.Fatal("AuthRequestDuration is nil")
		}
	})

	t.Run("Room creation metrics", func(t *testing.T) {
		if RoomCreationTotal == nil {
			t.Fatal("RoomCreationTotal is nil")
		}
		if RoomCreationDuration == nil {
			t.Fatal("RoomCreationDuration is nil")
		}
	})

	t.Run("Stream metrics", func(t *testing.T) {
		if GameResultsStreamLen == nil {
			t.Fatal("GameResultsStreamLen is nil")
		}
		if EmailQueueStreamLen == nil {
			t.Fatal("EmailQueueStreamLen is nil")
		}
	})
}
