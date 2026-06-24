package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// setupTestRedis starts a miniredis server for testing.
// If miniredis is not available, tests are skipped.
func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	// Try connecting to a local Redis for integration tests.
	// In CI, use a real Redis instance.
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}

	// Flush test DB
	rdb.FlushDB(ctx)

	return rdb
}

// TestIdempotencyMiddleware_CachesSuccessfulResponse verifies that the middleware
// automatically caches 2xx responses and replays them on subsequent requests
// with the same Idempotency-Key.
func TestIdempotencyMiddleware_CachesSuccessfulResponse(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	var handlerCalls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&handlerCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":"A1B2C3"}`))
	})

	mw := IdempotencyMiddleware(rdb)
	wrapped := mw(handler)

	// First request with Idempotency-Key
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)
	req1.Header.Set("Idempotency-Key", "test-key-001")
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: status = %d; want %d", rec1.Code, http.StatusOK)
	}
	if atomic.LoadInt32(&handlerCalls) != 1 {
		t.Fatalf("handler should be called exactly once; got %d", handlerCalls)
	}
	if rec1.Header().Get("X-Idempotent-Replayed") != "" {
		t.Fatal("first request should NOT have X-Idempotent-Replayed header")
	}

	// Second request with same Idempotency-Key — should return cached response
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)
	req2.Header.Set("Idempotency-Key", "test-key-001")
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("second request: status = %d; want %d", rec2.Code, http.StatusOK)
	}
	if atomic.LoadInt32(&handlerCalls) != 1 {
		t.Fatalf("handler should NOT be called again; got %d calls", handlerCalls)
	}
	if rec2.Header().Get("X-Idempotent-Replayed") != "true" {
		t.Fatal("second request should have X-Idempotent-Replayed: true")
	}
	if rec2.Body.String() != `{"code":"A1B2C3"}` {
		t.Fatalf("second request body = %q; want %q", rec2.Body.String(), `{"code":"A1B2C3"}`)
	}
}

// TestIdempotencyMiddleware_DifferentKeysBothExecute verifies that different
// Idempotency-Key values result in separate handler executions.
func TestIdempotencyMiddleware_DifferentKeysBothExecute(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	var handlerCalls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&handlerCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]int32{"count": count})
	})

	mw := IdempotencyMiddleware(rdb)
	wrapped := mw(handler)

	// First request with key A
	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set("Idempotency-Key", "key-A")
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)

	// Second request with key B — should execute handler again
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("Idempotency-Key", "key-B")
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)

	if atomic.LoadInt32(&handlerCalls) != 2 {
		t.Fatalf("different keys should result in 2 handler calls; got %d", handlerCalls)
	}
	if rec2.Header().Get("X-Idempotent-Replayed") != "" {
		t.Fatal("different key should NOT replay cached response")
	}
}

// TestIdempotencyMiddleware_NoKeyPassesThrough verifies that requests without
// an Idempotency-Key header pass through normally without caching.
func TestIdempotencyMiddleware_NoKeyPassesThrough(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	var handlerCalls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&handlerCalls, 1)
		w.WriteHeader(http.StatusOK)
	})

	mw := IdempotencyMiddleware(rdb)
	wrapped := mw(handler)

	// Two requests without Idempotency-Key — both should execute handler
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}

	if atomic.LoadInt32(&handlerCalls) != 2 {
		t.Fatalf("no key: handler should be called twice; got %d", handlerCalls)
	}
}

// TestIdempotencyMiddleware_Non2xxNotCached verifies that non-2xx responses
// are NOT cached, allowing the handler to be re-executed on retry.
func TestIdempotencyMiddleware_Non2xxNotCached(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	var handlerCalls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&handlerCalls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	})

	mw := IdempotencyMiddleware(rdb)
	wrapped := mw(handler)

	// First request returns 500
	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set("Idempotency-Key", "key-500")
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusInternalServerError {
		t.Fatalf("first request: status = %d; want %d", rec1.Code, http.StatusInternalServerError)
	}

	// Second request with same key — handler should be called again (not cached)
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("Idempotency-Key", "key-500")
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)

	if atomic.LoadInt32(&handlerCalls) != 2 {
		t.Fatalf("non-2xx should not be cached; handler should be called twice; got %d", handlerCalls)
	}
}

// TestSaveIdempotencyResponse verifies the SaveIdempotencyResponse function
// stores data correctly in Redis.
func TestSaveIdempotencyResponse(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	ctx := context.Background()
	key := "idem:test-save-key"

	err := SaveIdempotencyResponse(ctx, rdb, key, http.StatusOK, []byte(`{"result":"ok"}`), 5*time.Minute)
	if err != nil {
		t.Fatalf("SaveIdempotencyResponse failed: %v", err)
	}

	// Verify the data was stored
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get saved response: %v", err)
	}

	var cached idempotencyCachedResponse
	if err := json.Unmarshal([]byte(val), &cached); err != nil {
		t.Fatalf("failed to unmarshal cached response: %v", err)
	}

	if cached.StatusCode != http.StatusOK {
		t.Fatalf("cached status = %d; want %d", cached.StatusCode, http.StatusOK)
	}
	if cached.Body != `{"result":"ok"}` {
		t.Fatalf("cached body = %q; want %q", cached.Body, `{"result":"ok"}`)
	}
}

// TestGetIdempotencyKey verifies GetIdempotencyKey extracts the key from context.
func TestGetIdempotencyKey(t *testing.T) {
	// No key in context
	ctx := context.Background()
	if key := GetIdempotencyKey(ctx); key != "" {
		t.Fatalf("empty context should return empty key; got %q", key)
	}

	// Key in context
	ctx = context.WithValue(context.Background(), idempCtxKey{}, "idem:abc123")
	if key := GetIdempotencyKey(ctx); key != "idem:abc123" {
		t.Fatalf("key from context = %q; want %q", key, "idem:abc123")
	}
}
