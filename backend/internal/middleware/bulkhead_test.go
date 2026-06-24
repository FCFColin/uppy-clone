package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBulkhead_AllowsUnderLimit 验证配额未满时请求正常通过且调用下游。
func TestBulkhead_AllowsUnderLimit(t *testing.T) {
	b := NewBulkhead(2)
	called := 0
	handler := b.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d; want %d", i, w.Code, http.StatusOK)
		}
	}
	if called != 2 {
		t.Fatalf("handler called %d times; want 2", called)
	}
}

// TestBulkhead_RejectsWhenFull 验证配额耗尽时返回 503 且不调用下游处理。
// 使用 channel 确定性地等待首个请求占满配额后再发起第二个请求，避免时序竞争。
func TestBulkhead_RejectsWhenFull(t *testing.T) {
	b := NewBulkhead(1)
	acquired := make(chan struct{})
	release := make(chan struct{})
	handler := b.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(acquired) // signal that the semaphore has been acquired
		<-release       // hold the slot until released
		w.WriteHeader(http.StatusOK)
	}))

	// First request acquires the only slot and blocks.
	go func() {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
	}()

	// Wait until the first request has acquired the semaphore.
	<-acquired

	// Second request should be rejected with 503 immediately.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("second request status = %d; want %d", w2.Code, http.StatusServiceUnavailable)
	}
	if body := w2.Body.String(); !strings.Contains(body, "BULKHEAD_FULL") {
		t.Fatalf("response body = %q; want to contain BULKHEAD_FULL", body)
	}

	// Release the first request so the goroutine can exit.
	close(release)
}

// TestBulkhead_ReleasesAfterCompletion 验证请求完成后释放配额，后续请求可继续通过。
func TestBulkhead_ReleasesAfterCompletion(t *testing.T) {
	b := NewBulkhead(1)
	handler := b.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request completes and releases the slot.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status = %d; want %d", w1.Code, http.StatusOK)
	}

	// Second request should succeed because the slot was released.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second request status = %d; want %d (slot should be released)", w2.Code, http.StatusOK)
	}
}
