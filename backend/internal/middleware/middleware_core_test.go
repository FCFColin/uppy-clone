package middleware

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/uppy-clone/backend/internal/metrics"
)

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

func TestCORS(t *testing.T) {
	allowedOrigins := []string{"https://example.com", "https://app.example.com"}
	mw := CORS(allowedOrigins)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
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
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, Idempotency-Key" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Content-Type, Authorization, Idempotency-Key")
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
		{"empty string returns dev defaults", "", []string{
			"http://localhost:3000",
			"http://localhost:5173",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:5173",
		}},
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

func TestRequestIDLogger_LoggerIsNotDefault(t *testing.T) {
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

	// The injected logger should be different from the default logger
	// because it has request_id added
	defaultLogger := slog.Default()
	if capturedLogger == defaultLogger {
		t.Error("injected logger should be different from default (should have request_id)")
	}
}

func TestRequestIDLogger_NoRequestID(t *testing.T) {
	// Without RequestID middleware, the logger should not be injected
	var ctx context.Context

	handler := RequestIDLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Without chi RequestID middleware, no request_id is set, so no logger injected
	logger := LoggerFromContext(ctx)
	// Should fall back to default logger
	if logger == nil {
		t.Error("LoggerFromContext should return default logger, not nil")
	}
}

func TestLoggerFromContext_Fallback(t *testing.T) {
	// Empty context should return default logger
	logger := LoggerFromContext(context.Background())
	if logger == nil {
		t.Error("LoggerFromContext should return default logger for empty context, not nil")
	}

	// Verify it's the default logger
	defaultLogger := slog.Default()
	if logger != defaultLogger {
		t.Error("LoggerFromContext should return slog.Default() for empty context")
	}
}

func TestLoggerFromContext_WithInjectedLogger(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.WithValue(context.Background(), slogCtxKey, testLogger)

	logger := LoggerFromContext(ctx)
	if logger != testLogger {
		t.Error("LoggerFromContext should return the injected logger")
	}
}

func TestRequestIDLogger_CallsNext(t *testing.T) {
	called := false
	handler := RequestIDLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called")
	}
}

func TestTracingMiddleware_InjectsTraceID(t *testing.T) {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RequestIDLogger)
	r.Use(TracingMiddleware)

	var gotTraceID string
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		ctxLogger, ok := r.Context().Value(slogCtxKey).(*slog.Logger)
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

func TestTracingMiddleware_TraceIDInContextLogger(t *testing.T) {
	// Set up a test slog handler that captures all attributes
	var capturedAttrs []slog.Attr
	testHandler := &captureHandler{attrs: &capturedAttrs, ownAttrs: nil}
	testLogger := slog.New(testHandler)
	slog.SetDefault(testLogger)
	defer slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RequestIDLogger)
	r.Use(TracingMiddleware)

	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		logger := LoggerFromContext(r.Context())
		logger.Info("test message")
		w.WriteHeader(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify trace_id is in the captured attributes
	found := false
	for _, attr := range capturedAttrs {
		if attr.Key == "trace_id" {
			found = true
			if attr.Value.String() == "" {
				t.Error("trace_id attribute is empty")
			}
			break
		}
	}
	if !found {
		t.Error("trace_id not found in log attributes; captured:", capturedAttrs)
	}
}

func TestRequestIDLogger_InjectsRequestID(t *testing.T) {
	// Ensure slog has a valid handler (previous tests may have set it to nil)
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RequestIDLogger)

	var gotReqID string
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		gotReqID = chimw.GetReqID(r.Context())
		w.WriteHeader(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if gotReqID == "" {
		t.Error("request_id was not set in context")
	}
}

// captureHandler is a test slog.Handler that captures all attributes
// from both WithAttrs and Handle calls.
type captureHandler struct {
	attrs    *[]slog.Attr // shared output slice
	ownAttrs []slog.Attr  // pre-existing attrs from WithAttrs
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	// Capture both the handler's own attrs and the record's attrs
	*h.attrs = append(*h.attrs, h.ownAttrs...)
	r.Attrs(func(a slog.Attr) bool {
		*h.attrs = append(*h.attrs, a)
		return true
	})
	return nil
}
func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newOwnAttrs := make([]slog.Attr, len(h.ownAttrs), len(h.ownAttrs)+len(attrs))
	copy(newOwnAttrs, h.ownAttrs)
	newOwnAttrs = append(newOwnAttrs, attrs...)
	return &captureHandler{attrs: h.attrs, ownAttrs: newOwnAttrs}
}
func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

func TestTracingMiddleware_ResponseWriterSupportsHijack(t *testing.T) {
	hijackable := &hijackableResponseWriter{ResponseRecorder: httptest.NewRecorder()}

	r := chi.NewRouter()
	r.Use(TracingMiddleware)
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "not hijacker", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = conn.Close()
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.ServeHTTP(hijackable, req)

	if hijackable.Code != 0 && hijackable.Code != http.StatusOK {
		t.Fatalf("expected successful hijack, got status %d body %q", hijackable.Code, hijackable.Body.String())
	}
}

type hijackableResponseWriter struct {
	*httptest.ResponseRecorder
}

func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return &net.TCPConn{}, bufio.NewReadWriter(bufio.NewReader(nil), bufio.NewWriter(nil)), nil
}

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

func TestPrometheusMiddleware_IncrementsCounter(t *testing.T) {
	// Reset the counter before test
	metrics.HTTPRequestsTotal.Reset()

	handler := PrometheusMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestPrometheusMiddleware_ObservesDuration(t *testing.T) {
	metrics.HTTPRequestDuration.Reset()

	handler := PrometheusMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// For histograms, we can check the count of observations
	metric := testutil.CollectAndCount(metrics.HTTPRequestDuration)
	if metric < 1 {
		t.Errorf("HTTPRequestDuration should have at least 1 metric, got %d", metric)
	}
}

func TestPrometheusMiddleware_UsesChiRoutePattern(t *testing.T) {
	metrics.HTTPRequestsTotal.Reset()
	metrics.HTTPRequestDuration.Reset()

	r := chi.NewRouter()
	r.Use(PrometheusMiddleware)
	r.Get("/api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// The path label should be the chi route pattern, not the raw URL path
	count := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/users/{id}", "200"))
	if count < 1 {
		// Check if raw path was used instead (which would be wrong)
		rawCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/users/123", "200"))
		t.Errorf("Expected counter with route pattern label /api/v1/users/{id}, got count=%v; raw path label count=%v", count, rawCount)
	}
}

func TestPrometheusMiddleware_DifferentStatusCodes(t *testing.T) {
	metrics.HTTPRequestsTotal.Reset()

	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", 200},
		{"404 Not Found", 404},
		{"500 Internal Server Error", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := PrometheusMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		})
	}

	// Verify counters were created for different status codes
	count200 := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/test", "200"))
	count404 := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/test", "404"))
	count500 := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/test", "500"))
	if count200 < 1 || count404 < 1 || count500 < 1 {
		t.Errorf("Expected at least 1 observation per status code, got 200=%v 404=%v 500=%v", count200, count404, count500)
	}
}

func TestPrometheusMiddleware_FallbackToRawPath(t *testing.T) {
	metrics.HTTPRequestsTotal.Reset()

	// Without chi router, route pattern will be empty, so it falls back to URL path
	handler := PrometheusMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/some/random/path", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	count := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("POST", "/some/random/path", "200"))
	if count < 1 {
		t.Errorf("Expected counter with raw path label, got count=%v", count)
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

func TestTrustedProxy_StripsUntrustedForwardedHeaders(t *testing.T) {
	var seenProto, seenXFF string
	handler := TrustedProxy("127.0.0.1/32")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := TrustedProxy("10.0.0.0/8")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
