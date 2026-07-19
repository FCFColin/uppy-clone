package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/uppy-clone/backend/internal/metrics"
)

func TestCORS(t *testing.T) {
	allowedOrigins := []string{"https://example.com", "https://app.example.com"}
	mw := CORS(allowedOrigins)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	t.Run("allowed origin gets CORS headers", func(t *testing.T) {
		handler := mw(nextHandler)
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
	})
	t.Run("non-allowed origin gets no CORS headers", func(t *testing.T) {
		handler := mw(nextHandler)
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Origin", "https://evil.com")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
		}
	})
	t.Run("OPTIONS preflight returns correct headers and 204", func(t *testing.T) {
		handler := mw(nextHandler)
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
	})
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

func TestPrometheusMiddleware_IncrementsCounter(t *testing.T) {
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

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("sets X-Content-Type-Options nosniff", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
		}
	})
	t.Run("sets X-Frame-Options DENY", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("X-Frame-Options = %q, want DENY", got)
		}
	})
	t.Run("sets Referrer-Policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
			t.Errorf("Referrer-Policy = %q, want strict-origin-when-cross-origin", got)
		}
	})
	t.Run("sets Content-Security-Policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("Content-Security-Policy"); got == "" {
			t.Error("Content-Security-Policy should not be empty")
		}
	})
	t.Run("HSTS behavior", func(t *testing.T) {
		t.Run("set by default (ENABLE_HSTS not set)", func(t *testing.T) {
			_ = os.Unsetenv("ENABLE_HSTS")
			reinitSecurityConfig()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains; preload" {
				t.Errorf("HSTS = %q, want max-age=31536000; includeSubDomains; preload", got)
			}
		})
		t.Run("set when ENABLE_HSTS=true", func(t *testing.T) {
			_ = os.Setenv("ENABLE_HSTS", "true")
			defer func() { _ = os.Unsetenv("ENABLE_HSTS"); reinitSecurityConfig() }()
			reinitSecurityConfig()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains; preload" {
				t.Errorf("HSTS = %q, want max-age=31536000; includeSubDomains; preload", got)
			}
		})
		t.Run("not set when ENABLE_HSTS=false", func(t *testing.T) {
			_ = os.Setenv("ENABLE_HSTS", "false")
			defer func() { _ = os.Unsetenv("ENABLE_HSTS"); reinitSecurityConfig() }()
			reinitSecurityConfig()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
				t.Errorf("Strict-Transport-Security = %q, want empty", got)
			}
		})
	})
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

func TestGenerateNonce_RandFailure(t *testing.T) {
	prev := nonceRandRead
	nonceRandRead = func([]byte) (int, error) { return 0, errors.New("rand failed") }
	t.Cleanup(func() { nonceRandRead = prev })
	nonce := generateNonce()
	if len(nonce) != 32 {
		t.Fatalf("expected 32-char fallback nonce, got %d chars", len(nonce))
	}
}
