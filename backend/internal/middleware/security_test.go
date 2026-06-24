package middleware

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	t.Run("sets X-Content-Type-Options nosniff", func(t *testing.T) {
		rec := makeSecurityRequest()
		assertHeader(t, rec, "X-Content-Type-Options", "nosniff")
	})
	t.Run("sets X-Frame-Options DENY", func(t *testing.T) {
		rec := makeSecurityRequest()
		assertHeader(t, rec, "X-Frame-Options", "DENY")
	})
	t.Run("sets Referrer-Policy", func(t *testing.T) {
		rec := makeSecurityRequest()
		assertHeader(t, rec, "Referrer-Policy", "strict-origin-when-cross-origin")
	})
	t.Run("sets X-XSS-Protection", func(t *testing.T) {
		rec := makeSecurityRequest()
		assertHeader(t, rec, "X-XSS-Protection", "1; mode=block")
	})
	t.Run("sets Content-Security-Policy", func(t *testing.T) {
		rec := makeSecurityRequest()
		if got := rec.Header().Get("Content-Security-Policy"); got == "" {
			t.Error("Content-Security-Policy should not be empty")
		}
	})
	t.Run("HSTS behavior", func(t *testing.T) {
		testHSTSBehavior(t)
	})
	t.Run("calls next handler", func(t *testing.T) {
		testSecurityCallsNext(t)
	})
}

// makeSecurityRequest runs a request through SecurityHeaders and returns the recorder.
func makeSecurityRequest() *httptest.ResponseRecorder {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// assertHeader checks that a response header matches the expected value.
func assertHeader(t *testing.T, rec *httptest.ResponseRecorder, header, want string) {
	t.Helper()
	if got := rec.Header().Get(header); got != want {
		t.Errorf("%s = %q, want %q", header, got, want)
	}
}

// testHSTSBehavior verifies HSTS header behavior under different ENABLE_HSTS settings.
func testHSTSBehavior(t *testing.T) {
	t.Run("set by default (ENABLE_HSTS not set)", func(t *testing.T) {
		_ = os.Unsetenv("ENABLE_HSTS")
		rec := makeSecurityRequest()
		assertHeader(t, rec, "Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	})
	t.Run("set when ENABLE_HSTS=true", func(t *testing.T) {
		_ = os.Setenv("ENABLE_HSTS", "true")
		defer func() { _ = os.Unsetenv("ENABLE_HSTS") }()
		rec := makeSecurityRequest()
		assertHeader(t, rec, "Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	})
	t.Run("not set when ENABLE_HSTS=false", func(t *testing.T) {
		_ = os.Setenv("ENABLE_HSTS", "false")
		defer func() { _ = os.Unsetenv("ENABLE_HSTS") }()
		rec := makeSecurityRequest()
		if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("Strict-Transport-Security = %q, want empty", got)
		}
	})
}

// testSecurityCallsNext verifies the middleware calls the next handler in the chain.
func testSecurityCallsNext(t *testing.T) {
	called := false
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("next handler was not called")
	}
}
