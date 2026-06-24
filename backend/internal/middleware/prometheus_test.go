package middleware

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/uppy-clone/backend/internal/metrics"
)

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
