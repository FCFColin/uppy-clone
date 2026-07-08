package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupIdempotencyTest(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func conflictHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusConflict)
	_, _ = w.Write([]byte(`{"error":"conflict"}`))
}

func TestIdempotency_EmptyKeyPassThrough(t *testing.T) {
	rdb, _ := setupIdempotencyTest(t)
	mw := IdempotencyMiddleware(rdb)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestIdempotency_KeyTooLong(t *testing.T) {
	rdb, _ := setupIdempotencyTest(t)
	mw := IdempotencyMiddleware(rdb)

	longKey := strings.Repeat("a", 256)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", longKey)
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestIdempotency_ClaimAndCache2xx(t *testing.T) {
	rdb, mr := setupIdempotencyTest(t)
	mw := IdempotencyMiddleware(rdb)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", "test-key-1")
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	keys := mr.Keys()
	found := false
	for _, k := range keys {
		if strings.HasPrefix(k, "idem:") {
			found = true
			val, err := mr.Get(k)
			if err != nil {
				t.Fatalf("get cached key: %v", err)
			}
			var cached idempotencyCachedResponse
			if err := json.Unmarshal([]byte(val), &cached); err != nil {
				t.Fatalf("unmarshal cached: %v", err)
			}
			if cached.StatusCode != http.StatusOK {
				t.Fatalf("expected cached status 200, got %d", cached.StatusCode)
			}
		}
	}
	if !found {
		t.Fatal("no idempotency cache key found in redis")
	}
}

func TestIdempotency_Non2xxDeletesKey(t *testing.T) {
	rdb, mr := setupIdempotencyTest(t)
	mw := IdempotencyMiddleware(rdb)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", "test-key-2")
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(conflictHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	for _, k := range mr.Keys() {
		if strings.HasPrefix(k, "idem:") {
			t.Fatalf("expected key to be deleted on non-2xx, found %s", k)
		}
	}
}

func TestIdempotency_ReplayCachedResponse(t *testing.T) {
	rdb, _ := setupIdempotencyTest(t)
	mw := IdempotencyMiddleware(rdb)

	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set("Idempotency-Key", "test-key-3")
	rec1 := httptest.NewRecorder()
	mw(http.HandlerFunc(okHandler)).ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("Idempotency-Key", "test-key-3")
	rec2 := httptest.NewRecorder()
	mw(http.HandlerFunc(conflictHandler)).ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("replayed request: expected 200, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Idempotent-Replayed") != "true" {
		t.Fatal("expected X-Idempotent-Replayed header on replayed response")
	}
}

func TestIdempotency_DuplicateKeyReturns409(t *testing.T) {
	rdb, _ := setupIdempotencyTest(t)
	mw := IdempotencyMiddleware(rdb)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", "test-key-4")

	rec1 := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec2 := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rec2, r)
		if rec2.Code != http.StatusConflict {
			t.Errorf("concurrent request: expected 409, got %d", rec2.Code)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec1, req)
}
