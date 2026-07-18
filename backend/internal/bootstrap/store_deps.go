package bootstrap

import (
	"context"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/resilience"
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
		PostgresBreakerFactory: resilience.NewPostgresBreaker,
		RedisBreakerFactory:    resilience.NewRedisBreaker,
		DBRetryPolicy:          resilience.DefaultDBRetry(),
		RedisRetryPolicy:       resilience.DefaultRedisRetry(),
		MaybeRetryableFn:       resilience.MaybeRetryable,
		Tracer:                 telemetry.Tracer(),
		PoolMetrics:            PoolMetricsAdapter{},
		AuditLogFn: func(ctx context.Context, entry store.AuditEntry) {
			audit.Log(ctx, audit.AuditEntry{
				Action:    entry.Action,
				ActorType: entry.ActorType,
				ActorID:   entry.ActorID,
				ActorIP:   entry.ActorIP,
				Resource:  entry.Resource,
				Before:    entry.Before,
				After:     entry.After,
				RequestID: entry.RequestID,
				TraceID:   entry.TraceID,
			})
		},
	}
}
