package middleware

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestRequestIDLogger_InjectsLoggerIntoContext(t *testing.T) {
	var capturedLogger *slog.Logger

	handler := middleware.RequestID(
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

	handler := middleware.RequestID(
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
