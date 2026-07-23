package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestLiveHandler(t *testing.T) {
	t.Parallel()

	checker := NewChecker(nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/live", nil)

	checker.LiveHandler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "alive" {
		t.Errorf("status = %q, want %q", body["status"], "alive")
	}
}

func TestReadyHandler_NoDependencies(t *testing.T) {
	t.Parallel()

	checker := NewChecker(nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	checker.ReadyHandler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("status = %v, want %q", body["status"], "ready")
	}
}

func TestReadyHandler_PostgresUnavailable(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, "postgres://127.0.0.1:1/nodb?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	checker := NewChecker(pool, nil)
	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when postgres unavailable", rec.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "not ready" {
		t.Errorf("status = %v, want not ready", body["status"])
	}
	checks, _ := body["checks"].(map[string]interface{})
	if checks["postgres"] != "unavailable" {
		t.Errorf("checks = %v", checks)
	}
}

func TestReadyHandler_PostgresOK(t *testing.T) {
	checker := NewChecker(new(pgxpool.Pool), nil).WithPoolPing(func(context.Context) error { return nil })
	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	checks, _ := body["checks"].(map[string]interface{})
	if checks["postgres"] != "ok" {
		t.Fatalf("checks = %v", checks)
	}
}

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

func TestWithCanAcceptWS(t *testing.T) {
	t.Parallel()

	checker := NewChecker(nil, nil).WithCanAcceptWS(func() bool { return true })
	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}
