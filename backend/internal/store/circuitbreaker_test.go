package store

import (
	"testing"

	"github.com/sony/gobreaker/v2"
)

// TestNewBreakers verifies that each breaker constructor returns a non-nil
// breaker in the closed state.
func TestNewBreakers(t *testing.T) {
	tests := []struct {
		name    string
		breaker func() *gobreaker.CircuitBreaker[any]
	}{
		{"postgres", NewPostgresBreaker},
		{"redis", NewRedisBreaker},
		{"resend", NewResendBreaker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetBreakersForTesting()
			cb := tt.breaker()
			if cb == nil {
				t.Fatal("breaker returned nil")
			}
			if cb.State() != gobreaker.StateClosed {
				t.Fatalf("breaker should start in closed state, got %v", cb.State())
			}
		})
	}
}

// TestBreaker_ExecuteSuccess verifies that a closed breaker returns the result
// of the wrapped function unchanged.
func TestBreaker_ExecuteSuccess(t *testing.T) {
	tests := []struct {
		name    string
		breaker func() *gobreaker.CircuitBreaker[any]
		want    any
	}{
		{"postgres", NewPostgresBreaker, 42},
		{"redis", NewRedisBreaker, "redis-ok"},
		{"resend", NewResendBreaker, "resend-ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := tt.breaker()
			got, err := cb.Execute(func() (any, error) {
				return tt.want, nil
			})
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestBreaker_FailedExecution verifies that breaker execution surfaces the
// wrapped function's error.
func TestBreaker_FailedExecution(t *testing.T) {
	cb := NewPostgresBreaker()
	_, err := cb.Execute(func() (any, error) {
		return nil, errCBTest
	})
	if err == nil {
		t.Fatal("expected error from failed execution")
	}
}

// TestBreaker_OpensAfterFailures verifies that each breaker opens after its
// configured consecutive-failure threshold.
func TestBreaker_OpensAfterFailures(t *testing.T) {
	tests := []struct {
		name     string
		breaker  func() *gobreaker.CircuitBreaker[any]
		failures int
	}{
		{"postgres", NewPostgresBreaker, 6},
		{"redis", NewRedisBreaker, 6},
		{"resend", NewResendBreaker, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetBreakersForTesting()
			cb := tt.breaker()
			for i := 0; i < tt.failures; i++ {
				_, _ = cb.Execute(func() (any, error) {
					return nil, errCBTest
				})
			}
			if cb.State() != gobreaker.StateOpen {
				t.Fatalf("breaker should be open after %d consecutive failures, got %v", tt.failures, cb.State())
			}
		})
	}
}

func TestCircuitBreakerStateValue_UnknownState(t *testing.T) {
	if got := circuitBreakerStateValue(gobreaker.State(99)); got != -1 {
		t.Fatalf("unknown state = %v, want -1", got)
	}
}

var errCBTest = &cbTestError{}

type cbTestError struct{}

func (e *cbTestError) Error() string { return "test error" }
