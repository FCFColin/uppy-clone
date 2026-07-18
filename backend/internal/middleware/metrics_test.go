package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/uppy-clone/backend/internal/metrics"
)

// RecordAuthMetrics 必须把每次请求的 (endpoint, status_code) 维度都记录到
// Prometheus 指标里——运营 SLO 看板依赖这个。
//
// 这些测试不调用 t.Parallel()：metrics.AuthRequestTotal 是包级全局 counter，
// 每个测试都调用 Reset() 以获得干净基线。若并行执行，一个测试的 Reset 会
// 清掉另一个测试刚 Inc 的维度，导致偶发 FAIL。串行执行保证 Reset/Inc/断言
// 三步原子可见。详见 slim-tier1-ef-and-materialize Task 14.4 验证记录。

func TestRecordAuthMetrics_RecordsSuccessStatus(t *testing.T) {
	endpoint := "test-auth-success"
	metrics.AuthRequestTotal.Reset()
	mw := RecordAuthMetrics(endpoint)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/quickplay", nil)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues(endpoint, "200")); got != 1 {
		t.Fatalf("AuthRequestTotal[%s,200] = %f, want 1", endpoint, got)
	}
}

func TestRecordAuthMetrics_RecordsErrorStatus(t *testing.T) {
	endpoint := "test-auth-err"
	metrics.AuthRequestTotal.Reset()
	mw := RecordAuthMetrics(endpoint)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/quickplay", nil)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues(endpoint, "401")); got != 1 {
		t.Fatalf("AuthRequestTotal[%s,401] = %f, want 1", endpoint, got)
	}
	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues(endpoint, "200")); got != 0 {
		t.Fatalf("AuthRequestTotal[%s,200] = %f, want 0 (no 200 should be recorded)", endpoint, got)
	}
}

func TestRecordAuthMetrics_PassesRequestThrough(t *testing.T) {
	endpoint := "test-auth-passthrough"
	metrics.AuthRequestTotal.Reset()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/healthz", nil)
	rec := httptest.NewRecorder()
	RecordAuthMetrics(endpoint)(next).ServeHTTP(rec, req)

	if !called {
		t.Fatal("next handler was not called")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRecordAuthMetrics_StatusZeroRecordedAs200(t *testing.T) {
	// statusWriter.Status() returns 200 when no WriteHeader was called.
	// RecordAuth should observe a 200 in that case (handlers that just write body).
	endpoint := "test-auth-implicit-200"
	metrics.AuthRequestTotal.Reset()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/health", nil)
	rec := httptest.NewRecorder()
	RecordAuthMetrics(endpoint)(next).ServeHTTP(rec, req)

	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues(endpoint, "200")); got != 1 {
		t.Fatalf("AuthRequestTotal[%s,200] = %f, want 1 (implicit 200)", endpoint, got)
	}
}
