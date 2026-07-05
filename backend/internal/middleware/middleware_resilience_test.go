package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

// fakeRateLimiterStore is a test double for RateLimiterStore. It records every
// key passed to CheckRateLimit and returns the configured allow/err values.
type fakeRateLimiterStore struct {
	mu     sync.Mutex
	keys   []string
	allow  bool
	err    error
	called int
}

func (f *fakeRateLimiterStore) CheckRateLimit(_ context.Context, key string, maxCount int64, window time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, key)
	f.called++
	return f.allow, f.err
}

func (f *fakeRateLimiterStore) lastKey() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.keys) == 0 {
		return ""
	}
	return f.keys[len(f.keys)-1]
}

func newRequest(remoteAddr string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	return r
}

func newAuthRequest(remoteAddr, userID, nickname string) *http.Request {
	r := newRequest(remoteAddr)
	ctx := auth.WithAuthenticatedUser(r.Context(), userID, nickname)
	return r.WithContext(ctx)
}

// ─── rateLimitKey tests ─────────────────────────────────────────────

// TestRateLimitKey_AuthenticatedUsesUserID 验证认证请求的限流 key 包含
// user_id 而非仅 IP（修复"token cookie 名错误导致 userID 恒空"的缺陷）。
func TestRateLimitKey_AuthenticatedUsesUserID(t *testing.T) {
	r := newAuthRequest("1.2.3.4:5678", "user-abc", "alice")

	key := rateLimitKey(r, "registry:create", nil)

	if !strings.Contains(key, "user-abc") {
		t.Fatalf("authenticated key should contain user_id; got %q", key)
	}
	if !strings.HasPrefix(key, "registry:create:user-abc:") {
		t.Fatalf("authenticated key format = %q; want prefix %q", key, "registry:create:user-abc:")
	}
	if !strings.HasSuffix(key, "1.2.3.4") {
		t.Fatalf("authenticated key should still contain IP; got %q", key)
	}
}

// TestRateLimitKey_UnauthenticatedUsesIP 验证未认证请求回退到 IP 维度。
func TestRateLimitKey_UnauthenticatedUsesIP(t *testing.T) {
	r := newRequest("5.6.7.8:1234")

	key := rateLimitKey(r, "auth:quickplay", nil)

	want := "auth:quickplay:5.6.7.8"
	if key != want {
		t.Fatalf("unauthenticated key = %q; want %q", key, want)
	}
}

// TestRateLimitKey_DifferentUsersDifferentKeys 验证同一 IP 下不同用户
// 得到不同限流 key（用户级隔离生效）。
func TestRateLimitKey_DifferentUsersDifferentKeys(t *testing.T) {
	r1 := newAuthRequest("10.0.0.1:1", "user-1", "a")
	r2 := newAuthRequest("10.0.0.1:1", "user-2", "b")

	k1 := rateLimitKey(r1, "registry:create", nil)
	k2 := rateLimitKey(r2, "registry:create", nil)

	if k1 == k2 {
		t.Fatalf("different users must have different keys; both = %q", k1)
	}
}

// TestRateLimitKey_SessionCookieFallback 验证当 context 中无 userID 时，
// 回退到解析 "session" cookie 提取 userID。
func TestRateLimitKey_SessionCookieFallback(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, err := jwtMgr.SignToken("user-from-session", "alice")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	r := newRequest("1.2.3.4:5678")
	r.AddCookie(&http.Cookie{Name: "session", Value: token})

	key := rateLimitKey(r, "registry:create", jwtMgr)

	if !strings.Contains(key, "user-from-session") {
		t.Fatalf("key should contain userID from session cookie; got %q", key)
	}
	if !strings.HasPrefix(key, "registry:create:user-from-session:") {
		t.Fatalf("key format = %q; want prefix %q", key, "registry:create:user-from-session:")
	}
}

// TestRateLimitKey_QuickplayCookieFallback 验证当 context 中无 userID 且
// 无 session cookie 时，回退到解析 "quickplay" cookie 提取 userID。
func TestRateLimitKey_QuickplayCookieFallback(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, err := jwtMgr.SignToken("user-from-quickplay", "bob")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	r := newRequest("9.8.7.6:1234")
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	key := rateLimitKey(r, "auth:quickplay", jwtMgr)

	if !strings.Contains(key, "user-from-quickplay") {
		t.Fatalf("key should contain userID from quickplay cookie; got %q", key)
	}
	if !strings.HasPrefix(key, "auth:quickplay:user-from-quickplay:") {
		t.Fatalf("key format = %q; want prefix %q", key, "auth:quickplay:user-from-quickplay:")
	}
}

// TestRateLimitKey_NoAuthCookiesFallsBackToIP 验证无任何认证 cookie 时
// 回退到 IP 维度限流。
func TestRateLimitKey_NoAuthCookiesFallsBackToIP(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	r := newRequest("3.3.3.3:33")

	key := rateLimitKey(r, "auth:quickplay", jwtMgr)

	want := "auth:quickplay:3.3.3.3"
	if key != want {
		t.Fatalf("no cookies: key = %q; want %q", key, want)
	}
}

// TestRateLimitKey_OldTokenCookieIgnored 验证名为 "token" 的旧 cookie
// 不会被识别为有效认证 cookie（修复前的 bug：只读 "token" cookie）。
func TestRateLimitKey_OldTokenCookieIgnored(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, err := jwtMgr.SignToken("user-should-be-ignored", "eve")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	r := newRequest("4.4.4.4:44")
	r.AddCookie(&http.Cookie{Name: "token", Value: token})

	key := rateLimitKey(r, "auth:quickplay", jwtMgr)

	// "token" cookie should NOT be recognized; must fall back to IP-only
	want := "auth:quickplay:4.4.4.4"
	if key != want {
		t.Fatalf("old 'token' cookie should be ignored; key = %q; want %q", key, want)
	}
}

// TestRateLimitKey_SessionPreferredOverQuickplay 验证同时存在 session 和
// quickplay cookie 时，优先使用 session cookie 的 userID。
func TestRateLimitKey_SessionPreferredOverQuickplay(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	sessionToken, _ := jwtMgr.SignToken("session-user", "alice")
	quickplayToken, _ := jwtMgr.SignToken("quickplay-user", "bob")

	r := newRequest("5.5.5.5:55")
	r.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: quickplayToken})

	key := rateLimitKey(r, "registry:create", jwtMgr)

	if !strings.Contains(key, "session-user") {
		t.Fatalf("session cookie should take priority; got %q", key)
	}
	if strings.Contains(key, "quickplay-user") {
		t.Fatalf("quickplay userID should not appear when session is valid; got %q", key)
	}
}

// TestRateLimitKey_ContextTakesPriorityOverCookies 验证 auth context 中的
// userID 优先于 cookie 解析结果。
func TestRateLimitKey_ContextTakesPriorityOverCookies(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	// Context has one userID
	r := newAuthRequest("6.6.6.6:66", "context-user", "carol")

	// Cookie has a different userID
	cookieToken, _ := jwtMgr.SignToken("cookie-user", "dave")
	r.AddCookie(&http.Cookie{Name: "session", Value: cookieToken})

	key := rateLimitKey(r, "registry:create", jwtMgr)

	if !strings.Contains(key, "context-user") {
		t.Fatalf("context userID should take priority; got %q", key)
	}
	if strings.Contains(key, "cookie-user") {
		t.Fatalf("cookie userID should not appear when context is set; got %q", key)
	}
}

// TestRateLimitKey_NilJWTMgrSkipsCookieFallback 验证 jwtMgr 为 nil 时
// 跳过 cookie 解析，直接回退到 IP 维度。
func TestRateLimitKey_NilJWTMgrSkipsCookieFallback(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, _ := jwtMgr.SignToken("should-be-ignored", "eve")

	r := newRequest("7.7.7.7:77")
	r.AddCookie(&http.Cookie{Name: "session", Value: token})

	// Pass nil jwtMgr — cookie fallback should be skipped
	key := rateLimitKey(r, "auth:quickplay", nil)

	want := "auth:quickplay:7.7.7.7"
	if key != want {
		t.Fatalf("nil jwtMgr should skip cookie fallback; key = %q; want %q", key, want)
	}
}

// TestRateLimitKey_InvalidJWTCookieFallsBackToIP 验证 cookie 中包含
// 无效 JWT 时，回退到 IP 维度而非报错。
func TestRateLimitKey_InvalidJWTCookieFallsBackToIP(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	r := newRequest("8.8.8.8:88")
	r.AddCookie(&http.Cookie{Name: "session", Value: "invalid-jwt-token"})

	key := rateLimitKey(r, "auth:quickplay", jwtMgr)

	want := "auth:quickplay:8.8.8.8"
	if key != want {
		t.Fatalf("invalid JWT cookie should fall back to IP; key = %q; want %q", key, want)
	}
}

// ─── EndpointRateLimit middleware tests ──────────────────────────────

// TestEndpointRateLimit_AuthenticatedKeyedByUser 验证中间件对认证用户
// 使用包含 user_id 的 key 进行限流。
func TestEndpointRateLimit_AuthenticatedKeyedByUser(t *testing.T) {
	store := &fakeRateLimiterStore{allow: true}
	mw := EndpointRateLimit(store, "registry:create", nil)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := newAuthRequest("1.2.3.4:5678", "user-xyz", "bob")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler should be called when allowed")
	}
	if got := store.lastKey(); !strings.Contains(got, "user-xyz") {
		t.Fatalf("store key should contain user_id; got %q", got)
	}
}

// TestEndpointRateLimit_UnauthenticatedKeyedByIP 验证中间件对未认证请求
// 使用 IP-only key。
func TestEndpointRateLimit_UnauthenticatedKeyedByIP(t *testing.T) {
	store := &fakeRateLimiterStore{allow: true}
	mw := EndpointRateLimit(store, "auth:quickplay", nil)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := newRequest("9.9.9.9:99")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if got := store.lastKey(); got != "auth:quickplay:9.9.9.9" {
		t.Fatalf("unauthenticated store key = %q; want %q", got, "auth:quickplay:9.9.9.9")
	}
}

// TestEndpointRateLimit_DeniedReturns429 验证超限时返回 429 且不调用下游。
func TestEndpointRateLimit_DeniedReturns429(t *testing.T) {
	store := &fakeRateLimiterStore{allow: false}
	mw := EndpointRateLimit(store, "auth:quickplay", nil)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	r := newRequest("1.1.1.1:1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Fatal("downstream handler must NOT be called when rate limited")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; want %d", w.Code, http.StatusTooManyRequests)
	}
}

// TestEndpointRateLimit_FailOpenOnStoreError 验证非安全敏感端点 Redis 出错时
// 放行请求（fail-open 策略）。
func TestEndpointRateLimit_FailOpenOnStoreError(t *testing.T) {
	store := &fakeRateLimiterStore{allow: false, err: errors.New("redis down")}
	mw := EndpointRateLimit(store, "registry:create", nil) // not FailClosed

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := newRequest("2.2.2.2:2")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("fail-open: downstream handler should be called on store error")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("fail-open status = %d; want %d", w.Code, http.StatusOK)
	}
}

// TestEndpointRateLimit_FailClosedOnStoreError 验证安全敏感端点
// （FailClosed=true）Redis 出错时拒绝请求。
func TestEndpointRateLimit_FailClosedOnStoreError(t *testing.T) {
	store := &fakeRateLimiterStore{allow: false, err: errors.New("redis down")}
	mw := EndpointRateLimit(store, "auth:quickplay", nil) // FailClosed=true in config

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	r := newRequest("3.3.3.3:3")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Fatal("fail-closed: downstream handler must NOT be called on store error")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("fail-closed status = %d; want %d", w.Code, http.StatusTooManyRequests)
	}
}

// TestEndpointRateLimit_AdminLoginFailClosed 验证 admin:login 端点
// Redis 出错时也拒绝请求（FailClosed=true）。
func TestEndpointRateLimit_AdminLoginFailClosed(t *testing.T) {
	store := &fakeRateLimiterStore{allow: false, err: errors.New("redis down")}
	mw := EndpointRateLimit(store, "admin:login", nil) // FailClosed=true in config

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	r := newRequest("4.4.4.4:4")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Fatal("admin:login fail-closed: handler must NOT be called on store error")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("admin:login fail-closed status = %d; want %d", w.Code, http.StatusTooManyRequests)
	}
}

// ─── RateLimit (IP-only) tests ──────────────────────────────────────

// TestRateLimit_IPBasedStillWorks 验证 IP 维度限流（RateLimit）仍正常工作，
// 且不受 user_id context 影响（保持对现有 3 个端点的行为不变）。
func TestRateLimit_IPBasedStillWorks(t *testing.T) {
	store := &fakeRateLimiterStore{allow: true}
	mw := RateLimit(store, RateLimitConfig{MaxRequests: 5, Window: time.Minute})

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	// 即使 context 中带有 user_id，RateLimit 仍应按 IP 限流
	r := newAuthRequest("3.3.3.3:3", "user-ignored", "z")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler should be called when allowed")
	}
	if got := store.lastKey(); got != "3.3.3.3" {
		t.Fatalf("RateLimit should key by IP only; got %q", got)
	}
}

func TestRateLimit_DeniedReturns429(t *testing.T) {
	store := &fakeRateLimiterStore{allow: false}
	mw := RateLimit(store, RateLimitConfig{MaxRequests: 5, Window: time.Minute})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run when rate limited")
	}))

	r := newRequest("9.9.9.9:9")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}
}

func TestRateLimit_FailOpenOnStoreError(t *testing.T) {
	store := &fakeRateLimiterStore{err: errors.New("redis down")}
	mw := RateLimit(store, RateLimitConfig{MaxRequests: 5, Window: time.Minute})

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newRequest("8.8.8.8:8"))
	if !called {
		t.Fatal("handler should run when store errors (fail-open)")
	}
}

func TestResponseRecorder_WriteWithoutWriteHeader(t *testing.T) {
	base := httptest.NewRecorder()
	rec := newResponseRecorder(base)
	if _, err := rec.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if rec.statusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want 200 default", rec.statusCode)
	}
	if !rec.written {
		t.Error("written should be true after Write")
	}
	if base.Body.String() != "hello" {
		t.Errorf("body = %q", base.Body.String())
	}
}

func TestResponseRecorder_WriteHeaderTwice(t *testing.T) {
	base := httptest.NewRecorder()
	rec := newResponseRecorder(base)
	rec.WriteHeader(http.StatusCreated)
	rec.WriteHeader(http.StatusAccepted)
	if rec.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want first WriteHeader to win", rec.statusCode)
	}
	if base.Code != http.StatusCreated {
		t.Errorf("base status = %d", base.Code)
	}
}

func TestResponseRecorder_WriteAfterWriteHeader(t *testing.T) {
	base := httptest.NewRecorder()
	rec := newResponseRecorder(base)
	rec.WriteHeader(http.StatusAccepted)
	if _, err := rec.Write([]byte("data")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if rec.statusCode != http.StatusAccepted {
		t.Errorf("statusCode = %d", rec.statusCode)
	}
}

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

func TestIdempotencyMiddleware_KeyTooLong(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	mw := IdempotencyMiddleware(rdb)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run when key too long")
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", strings.Repeat("k", 256))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestIdempotencyMiddleware_InvalidCachedResponse(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	idemKey := "bad-cache-key"
	hash := sha256.Sum256([]byte(idemKey))
	redisKey := "idem:" + hex.EncodeToString(hash[:])
	ctx := context.Background()
	if err := rdb.Set(ctx, redisKey, "not-json", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}

	var called int32
	handler := IdempotencyMiddleware(rdb)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", idemKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if atomic.LoadInt32(&called) != 1 {
		t.Fatal("invalid cached payload should fall through to handler")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestEndpointRateLimit_UnknownEndpointUsesDefault(t *testing.T) {
	store := &fakeRateLimiterStore{allow: true}
	mw := EndpointRateLimit(store, "unknown:endpoint", nil)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("unknown endpoint should use default rate limit config")
	}
}

func TestIdempotencyMiddleware_SaveErrorStillReturnsResponse(t *testing.T) {
	rdb := setupTestRedis(t)
	_ = rdb.Close()

	var called int32
	handler := IdempotencyMiddleware(rdb)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", "save-error-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if atomic.LoadInt32(&called) != 1 {
		t.Fatal("handler should run even when save fails")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestSaveIdempotencyResponse_RedisError(t *testing.T) {
	rdb := setupTestRedis(t)
	_ = rdb.Close()
	err := SaveIdempotencyResponse(context.Background(), rdb, "idem:closed", 200, []byte("{}"), time.Minute)
	if err == nil {
		t.Fatal("expected error when redis client is closed")
	}
}

func TestSaveIdempotencyResponse_MarshalError(t *testing.T) {
	rdb := setupTestRedis(t)
	defer func() { _ = rdb.Close() }()

	prev := idempotencyJSONMarshal
	idempotencyJSONMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	t.Cleanup(func() { idempotencyJSONMarshal = prev })

	err := SaveIdempotencyResponse(context.Background(), rdb, "idem:marshal", 200, []byte("{}"), time.Minute)
	if err == nil {
		t.Fatal("expected marshal error")
	}
}
