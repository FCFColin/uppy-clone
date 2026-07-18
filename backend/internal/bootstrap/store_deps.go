package bootstrap

import (
	"context"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
)

// NewStoreDeps builds production store.Deps with real resilience, tracing,
// metrics, and audit logging. Passed to store constructors via variadic
// parameter.
//
// Previously duplicated as server.newStoreDeps and worker.newWorkerStoreDeps
// (30+ lines of byte-level identical code).
func NewStoreDeps() store.Deps {
	return store.Deps{
		PostgresBreakerFactory: store.NewPostgresBreaker,
		RedisBreakerFactory:    store.NewRedisBreaker,
		DBRetryPolicy:          store.DefaultDBRetry(),
		RedisRetryPolicy:       store.DefaultRedisRetry(),
		MaybeRetryableFn:       store.MaybeRetryable,
		Tracer:                 telemetry.Tracer(),
		PoolMetrics:            PoolMetricsAdapter{},
		AuditLogFn: func(ctx context.Context, entry store.AuditEntry) {
			audit.Log(ctx, entry)
		},
	}
}
