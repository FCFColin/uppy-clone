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

func TestRecordWSMessage(_ *testing.T) {
	metrics.WSMessageDuration.Reset()
	metrics.RecordWSMessage("tap", 100*time.Millisecond)
	// Observe increments histogram; just verify no panic and metric exists.
}

func TestRecordRoomLockHold(_ *testing.T) {
	metrics.RecordRoomLockHold("tick", 5*time.Millisecond)
}

func TestSetRoomOutboundQueueDepth(_ *testing.T) {
	metrics.SetRoomOutboundQueueDepth("ROOM1", 3)
}

func TestSetRoomPersistLag(_ *testing.T) {
	metrics.SetRoomPersistLag("ROOM1", 250*time.Millisecond)
}

func TestStatusWriter_DefaultStatusOK(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := metrics.NewStatusWriter(rec)
	if sw.Status() != http.StatusOK {
		t.Fatalf("default status = %d", sw.Status())
	}
}

func TestAuthRecorder_EndRecordsMetrics(t *testing.T) {
	metrics.AuthRequestTotal.Reset()
	rec := httptest.NewRecorder()
	authRec, sw := metrics.BeginAuth("logout", rec)
	sw.WriteHeader(http.StatusNoContent)
	authRec.End()

	if got := testutil.ToFloat64(metrics.AuthRequestTotal.WithLabelValues("logout", "204")); got != 1 {
		t.Fatalf("count = %v", got)
	}
}
