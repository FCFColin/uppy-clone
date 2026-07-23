package util

import (
	"context"
	"log/slog"
	"testing"
)

func TestWithLogger_RoundTrip(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(nil, nil))
	ctx := WithLogger(context.Background(), custom)
	got := LoggerFromContext(ctx)
	if got != custom {
		t.Error("expected injected logger")
	}
}

func TestLoggerFromContext_WrongTypeIgnored(t *testing.T) {
	// Adversarial: poisoned context value must not panic; fall back to default.
	ctx := context.WithValue(context.Background(), CtxKey{}, "not-a-logger")
	if LoggerFromContext(ctx) != slog.Default() {
		t.Error("wrong type should fall back to default")
	}
}
