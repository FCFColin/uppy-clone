package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/slogctx"
	"github.com/uppy-clone/backend/internal/telemetry"
)

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
		logger := slogctx.LoggerFromContext(r.Context()).With("trace_id", traceID)
		ctx = slogctx.WithLogger(ctx, logger)

		// Add http.route attribute using chi's RouteContext
		if routeCtx := chi.RouteContext(ctx); routeCtx != nil {
			if routePattern := routeCtx.RoutePattern(); routePattern != "" {
				span.SetAttributes(attribute.String("http.route", routePattern))
			}
		}

		// Add enduser.id if user_id is in context
		if userID, _, ok := auth.GetAuthenticatedUser(r); ok && userID != "" {
			span.SetAttributes(attribute.String("enduser.id", userID))
		}

		ww := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(ww, r.WithContext(ctx))

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
