package game

import (
	"time"

	"github.com/uppy-clone/backend/internal/metrics"
)

func recordRoomLock(operation string, start time.Time) {
	metrics.RecordRoomLockHold(operation, time.Since(start))
}
