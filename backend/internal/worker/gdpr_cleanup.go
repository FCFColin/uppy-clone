package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sethvargo/go-retry"

	"github.com/uppy-clone/backend/internal/resilience"
)

var (
	gdprCleanupRuns = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gdpr_cleanup_runs_total",
		Help: "Total number of GDPR cleanup runs.",
	})
	gdprDeletedUsers = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gdpr_deleted_users_total",
		Help: "Total number of users hard-deleted by GDPR cleanup.",
	})
)

func init() {
	prometheus.MustRegister(gdprCleanupRuns)
	prometheus.MustRegister(gdprDeletedUsers)
}

const defaultGDPRRetentionDays = 30
const defaultGDPRCleanupInterval = 24 * time.Hour

// userHardDeleter permanently removes soft-deleted users past retention.
type userHardDeleter interface {
	HardDeleteExpiredUsers(ctx context.Context, retentionDays int) (int64, error)
}

// GDPRCleanupWorker hard-deletes users past the GDPR retention window.
type GDPRCleanupWorker struct {
	db            userHardDeleter
	retentionDays int
	interval      time.Duration
}

// NewGDPRCleanupWorker creates a GDPR hard-delete worker.
func NewGDPRCleanupWorker(db userHardDeleter, retentionDays int, interval time.Duration) *GDPRCleanupWorker {
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
	gdprCleanupRuns.Inc()

	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var deleted int64
	if err := retry.Do(runCtx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		var err error
		deleted, err = w.db.HardDeleteExpiredUsers(ctx, w.retentionDays)
		return resilience.MaybeRetryable(err)
	}); err != nil {
		slog.Error("gdpr cleanup failed after retries", "error", err)
		return
	}
	if deleted > 0 {
		gdprDeletedUsers.Add(float64(deleted))
		slog.Info("gdpr cleanup completed", "deleted_users", deleted, "retention_days", w.retentionDays)
	}
}
