package middleware

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/telemetry"
	"github.com/uppy-clone/backend/internal/util"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

// TracingMiddleware creates an OpenTelemetry span for each HTTP request.
//
// Enterprise rationale: Every HTTP request should have a span that captures
// method, path, and status code. This enables distributed tracing dashboards
// to show request flow and latency breakdown.
// trace_id is injected into slog context so logs and traces can be correlated.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := telemetry.Tracer().Start(r.Context(), r.Method+" "+r.URL.Path,
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
			),
		)
		defer span.End()

		// Inject trace_id into slog context for log-trace correlation
		traceID := span.SpanContext().TraceID().String()
		logger := util.LoggerFromContext(r.Context()).With("trace_id", traceID)
		ctx = util.WithLogger(ctx, logger)

		// Add enduser.id if user_id is in context
		if userID, _, ok := auth.GetAuthenticatedUser(r); ok && userID != "" {
			span.SetAttributes(attribute.String("enduser.id", userID))
		}

		ww := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(ww, r.WithContext(ctx))

		// Route pattern is populated by chi during ServeHTTP; read it from the request context.
		if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
			if routePattern := routeCtx.RoutePattern(); routePattern != "" {
				span.SetAttributes(attribute.String("http.route", routePattern))
			}
		}

		span.SetAttributes(attribute.Int("http.status_code", ww.statusCode))
		if ww.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(ww.statusCode))
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("responseWriter does not implement http.Hijacker")
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// responseRecorder wraps http.ResponseWriter to capture the response body and status code.
type responseRecorder struct {
	http.ResponseWriter
	body       bytes.Buffer
	statusCode int
	written    bool
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // default if WriteHeader is never called
	}
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.written {
		return
	}
	r.statusCode = code
	r.written = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.statusCode = http.StatusOK
		r.written = true
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
