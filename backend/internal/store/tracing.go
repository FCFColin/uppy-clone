package store

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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
