package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/uppy-clone/backend/internal/store"
)

const defaultGDPRRetentionDays = 30
const defaultGDPRCleanupInterval = 24 * time.Hour

// GDPRCleanupWorker hard-deletes users past the GDPR retention window.
type GDPRCleanupWorker struct {
	db            *store.PostgresStore
	retentionDays int
	interval      time.Duration
}

// NewGDPRCleanupWorker creates a GDPR hard-delete worker.
func NewGDPRCleanupWorker(db *store.PostgresStore, retentionDays int, interval time.Duration) *GDPRCleanupWorker {
	if retentionDays <= 0 {
		retentionDays = defaultGDPRRetentionDays
	}
	if interval <= 0 {
		interval = defaultGDPRCleanupInterval
	}
	return &GDPRCleanupWorker{
		db:            db,
		retentionDays: retentionDays,
		interval:      interval,
	}
}

// Start runs the cleanup loop until ctx is canceled.
func (w *GDPRCleanupWorker) Start(ctx context.Context) {
	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *GDPRCleanupWorker) runOnce(ctx context.Context) {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	deleted, err := w.db.HardDeleteExpiredUsers(runCtx, w.retentionDays)
	if err != nil {
		slog.Error("gdpr cleanup failed", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("gdpr cleanup completed", "deleted_users", deleted, "retention_days", w.retentionDays)
	}
}
