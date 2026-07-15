package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

func TestReadyHandler_PostgresOKIntegration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://test:test@127.0.0.1:5432/testdb?sslmode=disable&connect_timeout=1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skip("postgres not available for readiness ok test")
	}
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skip("postgres ping failed")
	}

	checker := NewChecker(pool, nil)
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

func TestWithCanAcceptWS(t *testing.T) {
	t.Parallel()

	checker := NewChecker(nil, nil).WithCanAcceptWS(func() bool { return true })
	rec := httptest.NewRecorder()
	checker.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}
