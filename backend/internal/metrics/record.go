package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// statusWriter captures the HTTP status code written by handlers.
type statusWriter struct {
	http.ResponseWriter
	status int
}

// NewStatusWriter wraps a ResponseWriter to capture the written status code.
func NewStatusWriter(w http.ResponseWriter) *statusWriter {
	return &statusWriter{ResponseWriter: w}
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Status() int {
	if sw.status == 0 {
		return http.StatusOK
	}
	return sw.status
}

func statusLabel(code int) string {
	return strconv.Itoa(code)
}

// RecordAuth records auth SLO counters and latency histogram.
func RecordAuth(endpoint string, statusCode int, start time.Time) {
	status := statusLabel(statusCode)
	AuthRequestTotal.WithLabelValues(endpoint, status).Inc()
	AuthRequestDuration.WithLabelValues(endpoint).Observe(time.Since(start).Seconds())
}

// RecordRoomCreation records room creation outcomes.
func RecordRoomCreation(status string, start time.Time) {
	RoomCreationTotal.WithLabelValues(status).Inc()
	RoomCreationDuration.Observe(time.Since(start).Seconds())
}

// RecordWSConnection records WebSocket upgrade/join outcomes.
func RecordWSConnection(status string) {
	WSConnectionTotal.WithLabelValues(status).Inc()
}

// RecordGameTickDuration records a single game tick duration in seconds.
// audit-014: Uses seconds instead of milliseconds for Prometheus naming convention.
func RecordGameTickDuration(d time.Duration) {
	GameTickDuration.Observe(d.Seconds())
}

// RecordRoomLockHold records how long Room.mu was held for an operation.
func RecordRoomLockHold(operation string, d time.Duration) {
	RoomLockHoldSeconds.WithLabelValues(operation).Observe(d.Seconds())
}

// SetRoomPersistLag updates persist lag for a room.
func SetRoomPersistLag(roomCode string, lag time.Duration) {
	RoomPersistLagSeconds.WithLabelValues(roomCode).Set(lag.Seconds())
}
