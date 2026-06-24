package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/auth"
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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")

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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")

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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
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
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")

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
