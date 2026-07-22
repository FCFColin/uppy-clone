package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInitDBLogger_NilPoolNoOp(t *testing.T) {
	// Adversarial: nil pool must not panic or initialize dbLogger.
	old := dbLogger
	defer func() { dbLogger = old }()

	InitDBLogger(nil, "secret", RetryPolicy{})
	if dbLogger != nil {
		t.Fatal("InitDBLogger with nil pool should not initialize dbLogger")
	}
	CloseDBLogger()
}

func TestLog_StdoutOnlyWithoutDB(t *testing.T) {
	var buf bytes.Buffer
	old := auditLogger
	auditLogger = slog.New(slog.NewJSONHandler(&buf, nil))
	defer func() { auditLogger = old }()

	Log(context.Background(), AuditEntry{
		Action:   "test.action",
		ActorID:  "user-1",
		ActorIP:  "127.0.0.1",
		Resource: "test",
	})
	if !bytes.Contains(buf.Bytes(), []byte("test.action")) {
		t.Errorf("log output = %s", buf.String())
	}
}

func TestLog_AutoTraceID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	var buf bytes.Buffer
	old := auditLogger
	auditLogger = slog.New(slog.NewJSONHandler(&buf, nil))
	defer func() { auditLogger = old }()

	tracer := otel.Tracer("audit-test")
	ctx, span := tracer.Start(context.Background(), "audit-span")
	defer span.End()

	Log(ctx, AuditEntry{Action: "test.trace", ActorID: "u1"})
	traceID := span.SpanContext().TraceID().String()
	if traceID == "" {
		t.Fatal("expected non-empty trace ID from span")
	}
	if !bytes.Contains(buf.Bytes(), []byte(traceID)) {
		t.Fatalf("log output = %s, want trace_id %q", buf.String(), traceID)
	}
}

func TestAuditEntry_JSON(t *testing.T) {
	e := AuditEntry{Action: "x", Before: map[string]int{"a": 1}}
	b, err := json.Marshal(e)
	if err != nil || !bytes.Contains(b, []byte("before")) {
		t.Fatalf("marshal: %s, %v", b, err)
	}
}
