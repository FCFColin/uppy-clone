// Package slogctx provides context-aware structured logging.
package slogctx

import (
	"context"
	"log/slog"
)

// CtxKey is the context key for the request-scoped slog.Logger.
// Exported so multiple packages (middleware, auth) can inject/retrieve
// the contextual logger without circular imports.
type CtxKey struct{}

var ctxKey = CtxKey{}

// LoggerFromContext retrieves the request-scoped logger from context.
// Falls back to slog.Default() if no logger is found.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// WithLogger returns a new context with the given logger stored.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey, logger)
}
