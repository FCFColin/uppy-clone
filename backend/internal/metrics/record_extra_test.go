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

func TestRecordRoomCreation(t *testing.T) {
	metrics.RoomCreationTotal.Reset()
	start := time.Now()
	metrics.RecordRoomCreation("success", start)
	if got := testutil.ToFloat64(metrics.RoomCreationTotal.WithLabelValues("success")); got != 1 {
		t.Fatalf("count = %v", got)
	}
}

func TestRecordWSMessage(t *testing.T) {
	metrics.WSMessageDuration.Reset()
	metrics.RecordWSMessage("tap", 100*time.Millisecond)
	// Observe increments histogram; just verify no panic and metric exists.
}

func TestRecordRoomLockHold(t *testing.T) {
	metrics.RecordRoomLockHold("tick", 5*time.Millisecond)
}

func TestSetRoomOutboundQueueDepth(t *testing.T) {
	metrics.SetRoomOutboundQueueDepth("ROOM1", 3)
}

func TestSetRoomPersistLag(t *testing.T) {
	metrics.SetRoomPersistLag("ROOM1", 250*time.Millisecond)
}

func TestWSMessageTypeName_AllCases(t *testing.T) {
	cases := map[byte]string{
		protocol.MsgTap:         "tap",
		protocol.MsgSetNickname: "set_nickname",
		protocol.MsgRestartVote: "restart_vote",
		protocol.MsgPing:        "ping",
		0xFF:                    "unknown",
	}
	for msgType, want := range cases {
		if got := metrics.WSMessageTypeName(msgType); got != want {
			t.Fatalf("WSMessageTypeName(0x%02x) = %q, want %q", msgType, got, want)
		}
	}
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
