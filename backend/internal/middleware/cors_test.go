package middleware

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
