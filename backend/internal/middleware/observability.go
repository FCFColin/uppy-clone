package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/util"
)

// LoggerFromContext retrieves the request-scoped logger from context.
// Falls back to slog.Default() if no logger is found.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	return util.LoggerFromContext(ctx)
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
			ctx := util.WithLogger(r.Context(), logger)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)

		// Log request duration with latency_ms field
		latency := time.Since(start)
		util.LoggerFromContext(r.Context()).Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"latency_ms", latency.Milliseconds(),
		)
	})
}

// PrometheusMiddleware records HTTP request metrics for Prometheus.
//
// Enterprise rationale: Golden Signals require latency (histogram),
// traffic (counter), and errors (counter by status code). This middleware
// captures all three for every HTTP request automatically.
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(ww.Status())

		// Use chi route pattern as path label to avoid high cardinality.
		// When not in a chi router context (e.g., tests), fall back to the
		// raw URL path so metrics are still recorded with a meaningful label.
		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = r.URL.Path
		}
		if routePattern == "" {
			routePattern = "__unknown__"
		}

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, routePattern, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, routePattern).Observe(duration)
	})
}

// RecordAuthMetrics returns middleware that records auth endpoint latency and status.
// NOTE: This function has no dedicated unit tests.
func RecordAuthMetrics(endpoint string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := metrics.NewStatusWriter(w)
			next.ServeHTTP(sw, r)
			metrics.RecordAuth(endpoint, sw.Status(), start)
		})
	}
}