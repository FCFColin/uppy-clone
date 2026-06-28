package resilience

import (
	"testing"

	"github.com/sony/gobreaker/v2"
)

// ─── NewPostgresBreaker ──────────────────────────────────────────────

func TestNewPostgresBreaker(t *testing.T) {
	cb := NewPostgresBreaker()
	if cb == nil {
		t.Fatal("NewPostgresBreaker returned nil")
	}
}

func TestNewPostgresBreaker_InitialState(t *testing.T) {
	cb := NewPostgresBreaker()
	if cb.State() != gobreaker.StateClosed {
		t.Fatalf("breaker should start in closed state, got %v", cb.State())
	}
}

func TestNewPostgresBreaker_Name(t *testing.T) {
	cb := NewPostgresBreaker()
	// gobreaker doesn't expose name directly, but we can verify it executes
	_, err := cb.Execute(func() (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("closed breaker should allow execution, got error: %v", err)
	}
}

func TestNewPostgresBreaker_SuccessfulExecution(t *testing.T) {
	cb := NewPostgresBreaker()
	result, err := cb.Execute(func() (any, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Fatalf("expected 42, got %v", result)
	}
}

func TestNewPostgresBreaker_FailedExecution(t *testing.T) {
	cb := NewPostgresBreaker()
	_, err := cb.Execute(func() (any, error) {
		return nil, errTest
	})
	if err == nil {
		t.Fatal("expected error from failed execution")
	}
}

// ─── NewRedisBreaker ─────────────────────────────────────────────────

func TestNewRedisBreaker(t *testing.T) {
	cb := NewRedisBreaker()
	if cb == nil {
		t.Fatal("NewRedisBreaker returned nil")
	}
}

func TestNewRedisBreaker_InitialState(t *testing.T) {
	cb := NewRedisBreaker()
	if cb.State() != gobreaker.StateClosed {
		t.Fatalf("breaker should start in closed state, got %v", cb.State())
	}
}

func TestNewRedisBreaker_SuccessfulExecution(t *testing.T) {
	cb := NewRedisBreaker()
	result, err := cb.Execute(func() (any, error) {
		return "redis-ok", nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "redis-ok" {
		t.Fatalf("expected redis-ok, got %v", result)
	}
}

// ─── NewResendBreaker ────────────────────────────────────────────────

func TestNewResendBreaker(t *testing.T) {
	cb := NewResendBreaker()
	if cb == nil {
		t.Fatal("NewResendBreaker returned nil")
	}
}

func TestNewResendBreaker_InitialState(t *testing.T) {
	cb := NewResendBreaker()
	if cb.State() != gobreaker.StateClosed {
		t.Fatalf("breaker should start in closed state, got %v", cb.State())
	}
}

func TestNewResendBreaker_SuccessfulExecution(t *testing.T) {
	cb := NewResendBreaker()
	result, err := cb.Execute(func() (any, error) {
		return "resend-ok", nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "resend-ok" {
		t.Fatalf("expected resend-ok, got %v", result)
	}
}

// ─── Breaker opens after consecutive failures ────────────────────────

func TestPostgresBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewPostgresBreaker()

	// Trip the breaker with 6 consecutive failures (threshold is 5)
	for i := 0; i < 6; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, errTest
		})
	}

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("breaker should be open after 6 consecutive failures, got %v", cb.State())
	}
}

func TestRedisBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewRedisBreaker()

	for i := 0; i < 6; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, errTest
		})
	}

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("breaker should be open after 6 consecutive failures, got %v", cb.State())
	}
}

func TestResendBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewResendBreaker()

	// ResendBreaker trips after 3 consecutive failures
	for i := 0; i < 4; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, errTest
		})
	}

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("breaker should be open after 4 consecutive failures, got %v", cb.State())
	}
}

func TestOnStateChange_AllStates(t *testing.T) {
	onStateChange("test-breaker", gobreaker.StateClosed, gobreaker.StateHalfOpen)
	onStateChange("test-breaker", gobreaker.StateHalfOpen, gobreaker.StateOpen)
	onStateChange("test-breaker", gobreaker.StateOpen, gobreaker.StateClosed)
	onStateChange("test-breaker", gobreaker.StateClosed, gobreaker.StateClosed)
}

func TestCircuitBreakerStateValue_UnknownState(t *testing.T) {
	if got := circuitBreakerStateValue(gobreaker.State(99)); got != -1 {
		t.Fatalf("unknown state = %v, want -1", got)
	}
}

// ─── test error ──────────────────────────────────────────────────────

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }
