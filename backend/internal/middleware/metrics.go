package middleware

import (
	"net/http"
	"time"

	"github.com/uppy-clone/backend/internal/metrics"
)

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
