package metrics //nolint:revive // intentional package name

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

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

	RoomCreationDuration.Observe(0.1)
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
