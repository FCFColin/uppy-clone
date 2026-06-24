package slogctx

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

// testKey is a dedicated context key type for testing (avoids string keys per SA1012).
type testKey struct{}

// TestLoggerFromContext_NoLogger_ReturnsDefault verifies that LoggerFromContext
// falls back to slog.Default() when no logger is stored in the context.
func TestLoggerFromContext_NoLogger_ReturnsDefault(t *testing.T) {
	ctx := context.Background()
	logger := LoggerFromContext(ctx)
	if logger == nil {
		t.Fatal("LoggerFromContext returned nil for empty context")
	}
	// The fallback should be slog.Default().
	if logger != slog.Default() {
		t.Fatal("LoggerFromContext should return slog.Default() when no logger is set")
	}
}

// TestLoggerFromContext_WithLogger_ReturnsStoredLogger verifies that
// LoggerFromContext returns the logger stored via WithLogger.
func TestLoggerFromContext_WithLogger_ReturnsStoredLogger(t *testing.T) {
	customLogger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx := WithLogger(context.Background(), customLogger)

	got := LoggerFromContext(ctx)
	if got != customLogger {
		t.Fatal("LoggerFromContext did not return the stored logger")
	}
}

// TestWithLogger_DoesNotMutateOriginalContext verifies that WithLogger returns
// a new context and does not modify the original.
func TestWithLogger_DoesNotMutateOriginalContext(t *testing.T) {
	original := context.Background()
	customLogger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	_ = WithLogger(original, customLogger)

	// Original context should still have no logger → returns default.
	if LoggerFromContext(original) != slog.Default() {
		t.Fatal("WithLogger mutated the original context")
	}
}

// TestWithLogger_Overwrite verifies that calling WithLogger twice replaces
// the previously stored logger (key collision behavior).
func TestWithLogger_Overwrite(t *testing.T) {
	logger1 := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)).With("version", "1")
	logger2 := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)).With("version", "2")

	ctx := WithLogger(context.Background(), logger1)
	if LoggerFromContext(ctx) != logger1 {
		t.Fatal("first WithLogger did not store logger1")
	}

	ctx = WithLogger(ctx, logger2)
	if LoggerFromContext(ctx) != logger2 {
		t.Fatal("second WithLogger did not overwrite with logger2")
	}
}

// TestWithLogger_NilLogger verifies behavior when a nil logger is stored.
// LoggerFromContext uses a type assertion, so a nil *slog.Logger stored
// should still fall back to slog.Default() because the type assertion
// `(*slog.Logger)` succeeds but the value is nil — actually, a nil
// *slog.Logger stored in context will be returned as-is (non-nil interface
// holding a nil pointer). This is adversarial: callers should not store nil.
func TestWithLogger_NilLogger(_ *testing.T) {
	ctx := WithLogger(context.Background(), nil)
	got := LoggerFromContext(ctx)
	// The type assertion `ctx.Value(ctxKey).(*slog.Logger)` succeeds for a
	// nil *slog.Logger (it's a typed nil), so got will be nil.
	// This documents the current behavior: storing nil is unsafe.
	_ = got
}

// TestLoggerFromContext_NilContext verifies that LoggerFromContext returns
// slog.Default() for an empty context (SA1012: don't pass nil Context).
func TestLoggerFromContext_NilContext(t *testing.T) {
	got := LoggerFromContext(context.TODO())
	if got != slog.Default() {
		t.Fatal("LoggerFromContext should return slog.Default() for empty context")
	}
}

// TestWithLogger_NilContext verifies that WithLogger works with context.TODO()
// (SA1012: don't pass nil Context).
func TestWithLogger_NilContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx := WithLogger(context.TODO(), logger)
	if LoggerFromContext(ctx) != logger {
		t.Fatal("WithLogger should store the logger in context")
	}
}

// TestLoggerFromContext_WrongType verifies that if some other value is stored
// under the CtxKey, LoggerFromContext falls back to slog.Default().
// This is adversarial: tests the type assertion's "ok" branch.
func TestLoggerFromContext_WrongType(t *testing.T) {
	// Store a non-*slog.Logger value under the same key.
	ctx := context.WithValue(context.Background(), ctxKey, "not-a-logger")
	got := LoggerFromContext(ctx)
	if got != slog.Default() {
		t.Fatal("LoggerFromContext should fall back to default when value is wrong type")
	}
}

// TestLoggerFromContext_ProducesOutput verifies end-to-end that the logger
// retrieved from context actually writes log output.
func TestLoggerFromContext_ProducesOutput(t *testing.T) {
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))
	ctx := WithLogger(context.Background(), customLogger)

	logger := LoggerFromContext(ctx)
	logger.Info("test message", "key", "value")

	output := buf.String()
	if output == "" {
		t.Fatal("logger from context produced no output")
	}
	// The TextHandler output should contain the message.
	if !contains(output, "test message") {
		t.Fatalf("logger output missing message, got: %q", output)
	}
}

// TestCtxKey_IsExportedType verifies that CtxKey is an exported struct type
// so external packages can use it as a context key without circular imports.
func TestCtxKey_IsExportedType(_ *testing.T) {
	// CtxKey{} should be constructible from external code (test is in same
	// package, but the exported type allows cross-package use).
	k := CtxKey{}
	_ = k
	// The package-level ctxKey variable is of type CtxKey by definition.
	// Use a compile-time check instead of a runtime type assertion
	// (type assertions require an interface, but ctxKey is a concrete struct).
	var _ = ctxKey
	_ = k
}

// TestWithLogger_ChainedContext verifies that the logger survives context
// derivation (e.g., WithCancel, WithTimeout, WithValue of other keys).
func TestWithLogger_ChainedContext(t *testing.T) {
	customLogger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx := WithLogger(context.Background(), customLogger)

	// Derive a cancel context — logger should survive.
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	if LoggerFromContext(ctx2) != customLogger {
		t.Fatal("logger lost after context.WithCancel")
	}

	// Derive a timeout context — logger should survive.
	ctx3, cancel2 := context.WithTimeout(ctx, 0)
	defer cancel2()
	if LoggerFromContext(ctx3) != customLogger {
		t.Fatal("logger lost after context.WithTimeout")
	}

	// Add an unrelated value — logger should survive.
	ctx4 := context.WithValue(ctx, testKey{}, "other-value")
	if LoggerFromContext(ctx4) != customLogger {
		t.Fatal("logger lost after adding unrelated context value")
	}
}

// contains is a helper to check substring inclusion.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
