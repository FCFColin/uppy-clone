package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestReadyHandler_RedisUnavailable_Degraded(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = rdb.Close() })
	checker := NewChecker(nil, rdb)

	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 degraded", rec.Code)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
}

func TestReadyHandler_RedisOK(t *testing.T) {
	_, rdb := testutil.NewTestMiniredis(t)
	checker := NewChecker(nil, rdb)

	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}
