package bootstrap

import "time"

// GDPR cleanup defaults — shared by server.startWorkers and worker.runWorker
// to compute the GDPRCleanupWorker config from env values. The
// GDPRCleanupWorker constructor (worker.NewGDPRCleanupWorker) also applies
// these as a defensive safety net when called with zero values.
const (
	DefaultGDPRRetentionDays   = 30
	DefaultGDPRCleanupInterval = 24 * time.Hour
)

// ResolveGDPRConfig returns the retention days and cleanup interval, applying
// defaults when env-provided values are zero/negative. The caller uses these
// resolved values for both the GDPRCleanupWorker constructor call and the
// "gdpr cleanup worker started" log line, so the log reflects the effective
// config rather than the raw env value.
//
// Previously duplicated inline in server.startWorkers and worker.runWorker
// (three `if x == 0 { x = default }` blocks each).
func ResolveGDPRConfig(retentionDays, cleanupIntervalHours int) (int, time.Duration) {
	if retentionDays <= 0 {
		retentionDays = DefaultGDPRRetentionDays
	}
	interval := time.Duration(cleanupIntervalHours) * time.Hour
	if interval <= 0 {
		interval = DefaultGDPRCleanupInterval
	}
	return retentionDays, interval
}
