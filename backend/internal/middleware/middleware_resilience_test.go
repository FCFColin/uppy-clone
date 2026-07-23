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

func (f *fakeRateLimiterStore) CheckRateLimit(_ context.Context, key string, _ int64, _ time.Duration) (bool, error) {
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

// TestRateLimitKey_TableDriven 验证 rateLimitKey 在各种认证场景下的行为。
func TestRateLimitKey_TableDriven(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	sessionToken, _ := jwtMgr.SignToken("user-from-session", "alice")
	contextCookieToken, _ := jwtMgr.SignToken("cookie-user", "dave")

	tests := []struct {
		name            string
		remoteAddr      string
		authUserID      string
		authNickname    string
		cookies         []struct{ name, value string }
		useNilJWTMgr    bool
		op              string
		wantExact       string
		wantContains    []string
		wantNotContains []string
		wantPrefix      string
		wantSuffix      string
	}{
		{
			name:         "AuthenticatedUsesUserID",
			remoteAddr:   "1.2.3.4:5678",
			authUserID:   "user-abc",
			authNickname: "alice",
			op:           EndpointRegistryCreate,
			wantContains: []string{"user-abc"},
			wantPrefix:   "registry:create:user-abc:",
			wantSuffix:   "1.2.3.4",
		},
		{
			name:       "UnauthenticatedUsesIP",
			remoteAddr: "5.6.7.8:1234",
			op:         EndpointAuthQuickplay,
			wantExact:  "auth:quickplay:5.6.7.8",
		},
		{
			name:         "SessionCookieFallback",
			remoteAddr:   "1.2.3.4:5678",
			cookies:      []struct{ name, value string }{{"session", sessionToken}},
			op:           EndpointRegistryCreate,
			wantContains: []string{"user-from-session"},
			wantPrefix:   "registry:create:user-from-session:",
		},
		{
			name:            "ContextTakesPriorityOverCookies",
			remoteAddr:      "6.6.6.6:66",
			authUserID:      "context-user",
			authNickname:    "carol",
			cookies:         []struct{ name, value string }{{"session", contextCookieToken}},
			op:              EndpointRegistryCreate,
			wantContains:    []string{"context-user"},
			wantNotContains: []string{"cookie-user"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRequest(tt.remoteAddr)
			if tt.authUserID != "" {
				ctx := auth.WithAuthenticatedUser(r.Context(), tt.authUserID, tt.authNickname)
				r = r.WithContext(ctx)
			}
			for _, c := range tt.cookies {
				r.AddCookie(&http.Cookie{Name: c.name, Value: c.value})
			}
			var mgr = jwtMgr
			if tt.useNilJWTMgr {
				mgr = nil
			}
			key := rateLimitKey(r, tt.op, mgr)

			if tt.wantExact != "" && key != tt.wantExact {
				t.Fatalf("key = %q; want %q", key, tt.wantExact)
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(key, s) {
					t.Fatalf("key should contain %q; got %q", s, key)
				}
			}
			for _, s := range tt.wantNotContains {
				if strings.Contains(key, s) {
					t.Fatalf("key should NOT contain %q; got %q", s, key)
				}
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(key, tt.wantPrefix) {
				t.Fatalf("key format = %q; want prefix %q", key, tt.wantPrefix)
			}
			if tt.wantSuffix != "" && !strings.HasSuffix(key, tt.wantSuffix) {
				t.Fatalf("key should end with %q; got %q", tt.wantSuffix, key)
			}
		})
	}
}

// TestEndpointRateLimit_AuthenticatedKeyedByUser 验证中间件对认证用户
// 使用包含 user_id 的 key 进行限流。
func TestEndpointRateLimit_AuthenticatedKeyedByUser(t *testing.T) {
	store := &fakeRateLimiterStore{allow: true}
	mw := EndpointRateLimit(store, EndpointRegistryCreate, nil)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	mw := EndpointRateLimit(store, EndpointAuthQuickplay, nil)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	mw := EndpointRateLimit(store, EndpointAuthQuickplay, nil)

	called := false
	handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
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
	mw := EndpointRateLimit(store, EndpointRegistryCreate, nil) // not FailClosed

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
// （FailClosed=true）Redis 出错时拒绝请求。覆盖三类 FailClosed 配置：
//   - auth:quickplay（显式 FailClosed）
//   - admin:login（显式 FailClosed）
//   - unknown:endpoint（回退到 default，default 必须 FailClosed，handler-014）
func TestEndpointRateLimit_FailClosedOnStoreError(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		addr     string
	}{
		{"AuthQuickplay", EndpointAuthQuickplay, "3.3.3.3:3"},
		{"DefaultFallback", "unknown:endpoint", "5.5.5.5:5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeRateLimiterStore{allow: false, err: errors.New("redis down")}
			mw := EndpointRateLimit(store, c.endpoint, nil)

			called := false
			handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				called = true
			}))

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, newRequest(c.addr))

			if called {
				t.Fatal("fail-closed: downstream handler must NOT be called on store error")
			}
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("fail-closed status = %d; want %d", w.Code, http.StatusTooManyRequests)
			}
		})
	}
}

// TestRateLimit_IPBasedStillWorks 验证 IP 维度限流（RateLimit）仍正常工作，
// 且不受 user_id context 影响（保持对现有 3 个端点的行为不变）。
func TestRateLimit_IPBasedStillWorks(t *testing.T) {
	store := &fakeRateLimiterStore{allow: true}
	mw := RateLimit(store, RateLimitConfig{MaxRequests: 5, Window: time.Minute})

	called := false
	handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
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

	handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
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
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newRequest("8.8.8.8:8"))
	if !called {
		t.Fatal("handler should run when store errors (fail-open)")
	}
}

func TestEndpointRateLimit_UnknownEndpointUsesDefault(t *testing.T) {
	store := &fakeRateLimiterStore{allow: true}
	mw := EndpointRateLimit(store, "unknown:endpoint", nil)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("unknown endpoint should use default rate limit config")
	}
}

// TestRateLimit_FailClosedOnStoreError 验证基础 RateLimit 在 FailClosed=true
// 时，Redis 出错拒绝请求（v2-R-05）。
func TestRateLimit_FailClosedOnStoreError(t *testing.T) {
	store := &fakeRateLimiterStore{allow: false, err: errors.New("redis down")}
	mw := RateLimit(store, RateLimitConfig{
		MaxRequests: 10,
		Window:      time.Minute,
		FailClosed:  true,
	})

	called := false
	handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	r := newRequest("5.5.5.5:5")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Fatal("fail-closed: downstream handler must NOT be called on store error")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("fail-closed status = %d; want %d", w.Code, http.StatusTooManyRequests)
	}
}
