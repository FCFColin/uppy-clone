package worker

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sethvargo/go-retry"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/util"
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

// GDPRCleanupWorker hard-deletes users past the GDPR retention window.
type GDPRCleanupWorker struct {
	hardDelete    func(ctx context.Context, retentionDays int) (int64, error)
	retentionDays int
	interval      time.Duration
}

// NewGDPRCleanupWorker creates a GDPR hard-delete worker. db may be nil in
// tests that only inspect retention/interval defaults.
func NewGDPRCleanupWorker(db *store.UserRepository, retentionDays int, interval time.Duration) *GDPRCleanupWorker {
	if retentionDays <= 0 {
		retentionDays = defaultGDPRRetentionDays
	}
	if interval <= 0 {
		interval = defaultGDPRCleanupInterval
	}
	w := &GDPRCleanupWorker{
		retentionDays: retentionDays,
		interval:      interval,
	}
	if db != nil {
		w.hardDelete = db.HardDeleteExpiredUsers
	}
	return w
}

// Start runs the cleanup loop until ctx is canceled.
func (w *GDPRCleanupWorker) Start(ctx context.Context) {
	// v2-R-84: inject a worker-scoped logger so all downstream log lines carry
	// the worker tag without each call site repeating it.
	logger := util.LoggerFromContext(ctx).With("worker", "gdpr_cleanup")
	ctx = util.WithLogger(ctx, logger)

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
	logger := util.LoggerFromContext(ctx)
	gdprCleanupRuns.Inc()

	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var deleted int64
	if err := retry.Do(runCtx, store.DefaultDBRetry(), func(ctx context.Context) error {
		var err error
		deleted, err = w.hardDelete(ctx, w.retentionDays)
		return store.MaybeRetryable(err)
	}); err != nil {
		logger.Error("gdpr cleanup failed after retries", "error", err)
		return
	}
	if deleted > 0 {
		gdprDeletedUsers.Add(float64(deleted))
		logger.Info("gdpr cleanup completed", "deleted_users", deleted, "retention_days", w.retentionDays)
		audit.Log(ctx, audit.AuditEntry{
			Action:    "gdpr_hard_delete_executed",
			ActorType: audit.ActorTypeSystem,
			ActorID:   "system",
			Resource:  "gdpr/cleanup",
		})
	}
}
