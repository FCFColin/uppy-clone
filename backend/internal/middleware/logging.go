package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/uppy-clone/backend/internal/slogctx"
)

// LoggerFromContext retrieves the request-scoped logger from context.
// Falls back to slog.Default() if no logger is found.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	return slogctx.LoggerFromContext(ctx)
}

// RequestIDLogger injects the chi request_id into the slog context,
// so all log statements within a request carry the same request_id.
// If trace_id is already in the slog context (set by TracingMiddleware),
// it will be preserved. If not, it will be read from the OTel span.
//
// Enterprise rationale: Request ID correlation is fundamental to observability.
// Without it, logs from the same HTTP request cannot be grouped in ELK/Loki.
// This middleware bridges chi's RequestID with slog's contextual logging.
func RequestIDLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if reqID := middleware.GetReqID(r.Context()); reqID != "" {
			logger := slog.Default().With("request_id", reqID)
			ctx := slogctx.WithLogger(r.Context(), logger)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)

		// Log request duration with latency_ms field
		latency := time.Since(start)
		slogctx.LoggerFromContext(r.Context()).Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"latency_ms", latency.Milliseconds(),
		)
	})
}
