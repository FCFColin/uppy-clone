// Package util provides shared utilities.
package util

import (
	"context"
	"log/slog"
)

// CtxKey is the context key for the request-scoped slog.Logger.
// Exported so multiple packages (middleware, auth) can inject/retrieve
// the contextual logger without circular imports.
// audit-021: Previously had both an exported CtxKey type and an unexported ctxKey
// var of the same type, creating redundant symbols. Now CtxKey{} is used directly.
type CtxKey struct{}

// LoggerFromContext retrieves the request-scoped logger from context.
// Falls back to slog.Default() if no logger is found.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(CtxKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// WithLogger returns a new context with the given logger stored.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, CtxKey{}, logger)
}
