package store

import (
	"context"

	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ─── Base Repository ─────────────────────────────────────────────────

type baseRepository struct {
	pool pgPool
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps
}

func newBaseRepository(pool pgPool, deps Deps) baseRepository {
	return baseRepository{pool: pool, cb: deps.PostgresBreakerFactory(), deps: deps}
}

// withRetry wraps fn with the DB retry policy and circuit breaker.
// Read/write distinction is handled by MaybeRetryableFn + breaker MaxRequests,
// so both paths share the same implementation.
func (b *baseRepository) withRetry(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, b.deps.DBRetryPolicy, func(ctx context.Context) error {
		_, err := b.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return b.deps.MaybeRetryableFn(err)
	})
}

// withSpan starts a named tracing span, executes fn, and ends the span.
// Attributes are optional key-value pairs appended to the span.
func withSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, name,
		trace.WithAttributes(append([]attribute.KeyValue{
			attribute.String("db.system", "postgresql"),
		}, attrs...)...),
	)
	return ctx, span
}
