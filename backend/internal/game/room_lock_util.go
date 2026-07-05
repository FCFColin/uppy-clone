package game

import (
	"time"

	"github.com/uppy-clone/backend/internal/metrics"
)

func recordRoomLock(operation string, start time.Time) {
	metrics.RecordRoomLockHold(operation, time.Since(start))
}

func recordGameTickDuration(start time.Time) {
	metrics.RecordGameTickDuration(time.Since(start))
}
