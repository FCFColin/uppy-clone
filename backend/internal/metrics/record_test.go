package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestStatusWriter_DefaultOK(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := metrics.NewStatusWriter(rec)
	sw.WriteHeader(http.StatusNoContent)
	if got := sw.Status(); got != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", got, http.StatusNoContent)
	}
}

func TestRecordAuth_IncrementsCounter(t *testing.T) {
	metrics.AuthRequestTotal.Reset()
	start := time.Now()
	metrics.RecordAuth("quickplay", http.StatusOK, start)
	metrics.RecordAuth("quickplay", http.StatusInternalServerError, start)

	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues("quickplay", "200")); got != 1 {
		t.Fatalf("200 count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues("quickplay", "500")); got != 1 {
		t.Fatalf("500 count = %v, want 1", got)
	}
}

func TestBeginAuth_RecordsOnEnd(t *testing.T) {
	metrics.AuthRequestTotal.Reset()
	rec := httptest.NewRecorder()
	authRec, w := metrics.BeginAuth("check", rec)
	w.WriteHeader(http.StatusUnauthorized)
	authRec.End()

	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues("check", "401")); got != 1 {
		t.Fatalf("401 count = %v, want 1", got)
	}
}

func TestWSMessageTypeName(t *testing.T) {
	if got := metrics.WSMessageTypeName(protocol.MsgSetNickname); got != "set_nickname" {
		t.Fatalf("got %q, want set_nickname", got)
	}
}

func TestRecordWSConnection(t *testing.T) {
	metrics.WSConnectionTotal.Reset()
	metrics.RecordWSConnection("established")
	metrics.RecordWSConnection("rejected")
	if got := testutil.ToFloat64(metrics.WSConnectionTotal.WithLabelValues("established")); got != 1 {
		t.Fatalf("established = %v", got)
	}
}
