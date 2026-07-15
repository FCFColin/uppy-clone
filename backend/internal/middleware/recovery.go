package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/uppy-clone/backend/internal/slogctx"
)

// Recovery returns middleware that recovers from panics in the next handler.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// handler-018: Include request_id in panic recovery log for traceability.
				logger := slogctx.LoggerFromContext(r.Context())
				if logger == nil {
					logger = slog.Default()
				}
				logger.Error("http handler panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
					"method", r.Method,
					"request_id", GetRequestID(r.Context()),
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
