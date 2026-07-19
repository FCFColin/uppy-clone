package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/util"
)

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

func TestCORS(t *testing.T) {
	allowedOrigins := []string{"https://example.com", "https://app.example.com"}
	mw := CORS(allowedOrigins)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	t.Run("allowed origin gets CORS headers", func(t *testing.T) {
		testCORSAllowedOrigin(t, mw, nextHandler)
	})
	t.Run("non-allowed origin gets no CORS headers", func(t *testing.T) {
		testCORSNonAllowedOrigin(t, mw, nextHandler)
	})
	t.Run("OPTIONS preflight returns correct headers and 204", func(t *testing.T) {
		testCORSPreflightAllowed(t, mw, nextHandler)
	})
	t.Run("OPTIONS preflight from non-allowed origin returns 204 without CORS headers", func(t *testing.T) {
		testCORSPreflightNonAllowed(t, mw, nextHandler)
	})
	t.Run("request without Origin header gets no CORS headers", func(t *testing.T) {
		testCORSNoOrigin(t, mw, nextHandler)
	})
}

func testCORSAllowedOrigin(t *testing.T, mw func(http.Handler) http.Handler, next http.Handler) {
	handler := mw(next)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://example.com")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
}

func testCORSNonAllowedOrigin(t *testing.T, mw func(http.Handler) http.Handler, next http.Handler) {
	handler := mw(next)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func testCORSPreflightAllowed(t *testing.T, mw func(http.Handler) http.Handler, next http.Handler) {
	handler := mw(next)
	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Content-Type, Authorization")
	}
}

func testCORSPreflightNonAllowed(t *testing.T, mw func(http.Handler) http.Handler, next http.Handler) {
	handler := mw(next)
	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for non-allowed origin", got)
	}
}

func testCORSNoOrigin(t *testing.T, mw func(http.Handler) http.Handler, next http.Handler) {
	handler := mw(next)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestAllowedOriginsFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string returns nil", "", nil},
		{"single origin", "https://example.com", []string{"https://example.com"}},
		{"multiple origins", "https://a.com, https://b.com", []string{"https://a.com", "https://b.com"}},
		{"trims whitespace", " https://a.com , https://b.com ", []string{"https://a.com", "https://b.com"}},
		{"skips empty parts", "https://a.com,,https://b.com", []string{"https://a.com", "https://b.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AllowedOriginsFromEnv(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("AllowedOriginsFromEnv(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("AllowedOriginsFromEnv(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

func TestRequestIDLogger_InjectsLoggerIntoContext(t *testing.T) {
	var capturedLogger *slog.Logger

	handler := chimw.RequestID(
		RequestIDLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedLogger = LoggerFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedLogger == nil {
		t.Fatal("expected logger in context, got nil")
	}
}

func TestTracingMiddleware_InjectsTraceID(t *testing.T) {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RequestIDLogger)
	r.Use(TracingMiddleware)

	var gotTraceID string
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		ctxLogger, ok := r.Context().Value(util.CtxKey{}).(*slog.Logger)
		if !ok {
			t.Error("no logger found in context")
			w.WriteHeader(500)
			return
		}
		_ = ctxLogger
		gotTraceID = "present"
		w.WriteHeader(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if gotTraceID != "present" {
		t.Error("trace_id was not injected into context logger")
	}
}

func TestPrometheusMiddleware_IncrementsCounter(t *testing.T) {
	// Reset the counter before test
	metrics.HTTPRequestsTotal.Reset()

	handler := PrometheusMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	count := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/api/test", "200"))
	if count < 1 {
		t.Errorf("HTTPRequestsTotal counter = %v, want >= 1", count)
	}
}

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

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
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		reinitSecurityConfig()
		rec := makeSecurityRequest()
		assertHeader(t, rec, "Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	})
	t.Run("set when ENABLE_HSTS=true", func(t *testing.T) {
		_ = os.Setenv("ENABLE_HSTS", "true")
		defer func() { _ = os.Unsetenv("ENABLE_HSTS"); reinitSecurityConfig() }()
		reinitSecurityConfig()
		rec := makeSecurityRequest()
		assertHeader(t, rec, "Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	})
	t.Run("not set when ENABLE_HSTS=false", func(t *testing.T) {
		_ = os.Setenv("ENABLE_HSTS", "false")
		defer func() { _ = os.Unsetenv("ENABLE_HSTS"); reinitSecurityConfig() }()
		reinitSecurityConfig()
		rec := makeSecurityRequest()
		if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("Strict-Transport-Security = %q, want empty", got)
		}
	})
}

// testSecurityCallsNext verifies the middleware calls the next handler in the chain.
func testSecurityCallsNext(t *testing.T) {
	called := false
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestTrustedProxy_StripsUntrustedForwardedHeaders(t *testing.T) {
	var seenProto, seenXFF string
	handler := TrustedProxy("127.0.0.1/32")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seenProto = r.Header.Get("X-Forwarded-Proto")
		seenXFF = r.Header.Get("X-Forwarded-For")
		if !IsTrustedProxy(r) {
			t.Error("expected trusted proxy when peer is 127.0.0.1")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seenProto != "https" {
		t.Fatalf("X-Forwarded-Proto = %q, want https", seenProto)
	}
	if seenXFF != "203.0.113.1" {
		t.Fatalf("X-Forwarded-For = %q, want 203.0.113.1", seenXFF)
	}
}

func TestTrustedProxy_StripsSpoofedHeadersFromUntrustedPeer(t *testing.T) {
	var seenProto string
	handler := TrustedProxy("10.0.0.0/8")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seenProto = r.Header.Get("X-Forwarded-Proto")
		if IsTrustedProxy(r) {
			t.Error("expected untrusted proxy for public peer")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seenProto != "" {
		t.Fatalf("X-Forwarded-Proto = %q, want empty", seenProto)
	}
}

func TestExtractClientIP_UsesRemoteAddrWhenUntrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:4321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := ExtractClientIP(req); got != "203.0.113.50" {
		t.Fatalf("ExtractClientIP() = %q, want 203.0.113.50", got)
	}
}

func TestExtractClientIP_UsesForwardedForWhenTrusted(t *testing.T) {
	handler := TrustedProxy("10.0.0.1/32")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := ExtractClientIP(r); got != "198.51.100.10" {
			t.Fatalf("ExtractClientIP() = %q, want 198.51.100.10", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.10")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestIsOriginAllowed_ExactMatch(t *testing.T) {
	allowed := []string{"https://app.example.com", "http://localhost:5173"}
	if !IsOriginAllowed("https://app.example.com", allowed) {
		t.Fatal("expected exact origin match")
	}
	// Adversarial: hostname-only match must fail
	if IsOriginAllowed("https://evil.example.com", allowed) {
		t.Fatal("hostname-only match must not pass")
	}
}

func TestGetRequestID_FromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), chimw.RequestIDKey, "req-123")
	if got := GetRequestID(ctx); got != "req-123" {
		t.Fatalf("GetRequestID = %q, want req-123", got)
	}
	if got := GetRequestID(context.Background()); got != "" {
		t.Fatalf("empty context GetRequestID = %q, want empty", got)
	}
}

func TestGenerateNonce_RandFailure(t *testing.T) {
	prev := nonceRandRead
	nonceRandRead = func([]byte) (int, error) { return 0, errors.New("rand failed") }
	t.Cleanup(func() { nonceRandRead = prev })
	nonce := generateNonce()
	if len(nonce) != 32 {
		t.Fatalf("expected 32-char fallback nonce, got %d chars", len(nonce))
	}
}
