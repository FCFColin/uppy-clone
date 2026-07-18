package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/uppy-clone/backend/internal/metrics"
)

func TestRecordRoomCreation(t *testing.T) {
	metrics.RoomCreationTotal.Reset()
	start := time.Now()
	metrics.RecordRoomCreation("success", start)
	if got := testutil.ToFloat64(metrics.RoomCreationTotal.WithLabelValues("success")); got != 1 {
		t.Fatalf("count = %v", got)
	}
}

func TestStatusWriter_DefaultStatusOK(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := metrics.NewStatusWriter(rec)
	if sw.Status() != http.StatusOK {
		t.Fatalf("default status = %d", sw.Status())
	}
}


