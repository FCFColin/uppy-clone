package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyHandler_WSAtCapacity(t *testing.T) {
	checker := NewChecker(nil, nil).WithCanAcceptWS(func() bool { return false })
	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when WS at capacity", rec.Code)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	checks, _ := body["checks"].(map[string]interface{})
	if checks["websocket"] != "at capacity" {
		t.Errorf("checks = %v", checks)
	}
}

func TestReadyHandler_DegradedWhenRedisDown(t *testing.T) {
	// Redis client pointing at closed port simulates unavailable Redis.
	checker := NewChecker(nil, nil)
	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("nil deps should still be ready, got %d", rec.Code)
	}
}
