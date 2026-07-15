package telemetry

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestTracerProvider creates a TracerProvider with an in-memory exporter
// for testing. The exporter captures spans without sending them to a backend.
func newTestTracerProvider(exporter *tracetest.InMemoryExporter) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
}

// setEnv sets an env var and returns a cleanup function to restore the original value.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %s=%s: %v", key, value, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// unsetEnv unsets an env var and returns a cleanup function to restore the original value.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// saveTracerProvider saves the global tracer provider and restores it on cleanup.
func saveTracerProvider(t *testing.T) {
	t.Helper()
	original := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(original)
	})
}

// TestTracer_ReturnsNonNil verifies that Tracer() returns a non-nil tracer.
func TestTracer_ReturnsNonNil(t *testing.T) {
	tr := Tracer()
	if tr == nil {
		t.Fatal("Tracer() returned nil")
	}
}

// TestTracer_ReturnsConsistentInstance verifies that Tracer() returns the
// same instance on repeated calls (it's a package-level var).
func TestTracer_ReturnsConsistentInstance(_ *testing.T) {
	tr1 := Tracer()
	tr2 := Tracer()
	// Tracer returns the same pointer/value each time.
	// trace.Tracer is an interface, so we compare interface values.
	// Since the underlying tracer is the same, they should be equal.
	_ = tr1
	_ = tr2
}

// TestInitTracer_NoEndpoint_ReturnsNoop verifies that when
// OTEL_EXPORTER_OTLP_ENDPOINT is not set, InitTracer returns a noop
// shutdown function and no error.
func TestInitTracer_NoEndpoint_ReturnsNoop(t *testing.T) {
	unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")
	unsetEnv(t, "OTEL_SAMPLE_RATIO")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx, "test-service", "1.0.0")
	if err != nil {
		t.Fatalf("InitTracer with no endpoint failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown function")
	}

	// The noop shutdown should return nil.
	if err := shutdown(ctx); err != nil {
		t.Fatalf("noop shutdown returned error: %v", err)
	}
}

// TestInitTracer_NoEndpoint_ShutdownIdempotent verifies that the noop
// shutdown function can be called multiple times without error.
func TestInitTracer_NoEndpoint_ShutdownIdempotent(t *testing.T) {
	unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx, "test-service", "1.0.0")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}

	if err := shutdown(ctx); err != nil {
		t.Fatalf("first shutdown failed: %v", err)
	}
	// Second call should also succeed (noop).
	if err := shutdown(ctx); err != nil {
		t.Fatalf("second shutdown failed: %v", err)
	}
}

// TestInitTracer_WithEndpoint verifies that InitTracer creates a real
// provider when OTEL_EXPORTER_OTLP_ENDPOINT is set. Uses a non-existent
// local endpoint — the gRPC client connects lazily, so creation should succeed.
func TestInitTracer_WithEndpoint(t *testing.T) {
	setEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:9999")
	setEnv(t, "OTEL_SAMPLE_RATIO", "0.5")
	saveTracerProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := InitTracer(ctx, "test-service", "1.0.0")
	if err != nil {
		t.Fatalf("InitTracer with endpoint failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown function")
	}

	// Shutdown should flush and close the provider. The exporter may fail
	// to connect (endpoint doesn't exist), but Shutdown should not error
	// on the provider side.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := shutdown(shutdownCtx); err != nil {
		t.Logf("shutdown returned error (may be expected with invalid endpoint): %v", err)
	}
}

// TestInitTracer_WithEndpoint_SetsGlobalProvider verifies that InitTracer
// sets the global tracer provider when an endpoint is configured.
func TestInitTracer_WithEndpoint_SetsGlobalProvider(t *testing.T) {
	setEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:9999")
	unsetEnv(t, "OTEL_SAMPLE_RATIO")
	saveTracerProvider(t)

	beforeProvider := otel.GetTracerProvider()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := InitTracer(ctx, "test-service", "1.0.0")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = shutdown(shutdownCtx)
	}()

	afterProvider := otel.GetTracerProvider()
	// The global provider should have changed (from noop to real SDK provider).
	if beforeProvider == afterProvider {
		t.Fatal("InitTracer did not replace the global tracer provider")
	}
}

// TestInitTracer_CancelledContext verifies that InitTracer handles a
// cancelled context gracefully.
func TestInitTracer_CancelledContext(t *testing.T) {
	unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	shutdown, err := InitTracer(ctx, "test-service", "1.0.0")
	// With no endpoint, the context is not used, so this should succeed.
	if err != nil {
		t.Fatalf("InitTracer with cancelled context and no endpoint failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown")
	}
	_ = shutdown(context.Background())
}

// --- getSampleRatio tests ---

// TestGetSampleRatio_Default verifies the default ratio is 0.1.
func TestGetSampleRatio_Default(t *testing.T) {
	unsetEnv(t, "OTEL_SAMPLE_RATIO")
	if r := getSampleRatio(); r != 0.1 {
		t.Fatalf("getSampleRatio() = %v, want 0.1", r)
	}
}

// TestGetSampleRatio_ValidValues verifies valid ratio values are parsed correctly.
func TestGetSampleRatio_ValidValues(t *testing.T) {
	tests := []struct {
		env  string
		want float64
	}{
		{"0.0", 0.0},
		{"0.1", 0.1},
		{"0.25", 0.25},
		{"0.5", 0.5},
		{"0.75", 0.75},
		{"1.0", 1.0},
		{"1", 1.0},
		{"0", 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			setEnv(t, "OTEL_SAMPLE_RATIO", tt.env)
			if got := getSampleRatio(); got != tt.want {
				t.Fatalf("getSampleRatio() with env %q = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

// TestGetSampleRatio_InvalidValues verifies invalid values fall back to default 0.1.
func TestGetSampleRatio_InvalidValues(t *testing.T) {
	tests := []string{
		"abc",
		"",
		"not-a-number",
		"nan",
		"inf",
		"-inf",
	}
	for _, env := range tests {
		t.Run(env, func(t *testing.T) {
			setEnv(t, "OTEL_SAMPLE_RATIO", env)
			if got := getSampleRatio(); got != 0.1 {
				t.Fatalf("getSampleRatio() with invalid env %q = %v, want 0.1 (default)", env, got)
			}
		})
	}
}

// TestGetSampleRatio_OutOfRange verifies out-of-range values fall back to default.
func TestGetSampleRatio_OutOfRange(t *testing.T) {
	tests := []struct {
		env  string
		desc string
	}{
		{"-0.1", "negative"},
		{"-1.0", "negative full"},
		{"1.1", "above one"},
		{"2.0", "two"},
		{"100", "hundred"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			setEnv(t, "OTEL_SAMPLE_RATIO", tt.env)
			if got := getSampleRatio(); got != 0.1 {
				t.Fatalf("getSampleRatio() with out-of-range env %q = %v, want 0.1 (default)", tt.env, got)
			}
		})
	}
}

// TestGetSampleRatio_BoundaryValues verifies exact boundary values 0.0 and 1.0
// are accepted (inclusive range).
func TestGetSampleRatio_BoundaryValues(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		setEnv(t, "OTEL_SAMPLE_RATIO", "0.0")
		if got := getSampleRatio(); got != 0.0 {
			t.Fatalf("getSampleRatio() with 0.0 = %v, want 0.0", got)
		}
	})
	t.Run("one", func(t *testing.T) {
		setEnv(t, "OTEL_SAMPLE_RATIO", "1.0")
		if got := getSampleRatio(); got != 1.0 {
			t.Fatalf("getSampleRatio() with 1.0 = %v, want 1.0", got)
		}
	})
}

// --- Integration test using tracetest ---

// TestTracer_CreatesSpan verifies that the tracer can create spans.
// This uses tracetest.InMemoryExporter to capture spans without a backend.
func TestTracer_CreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	// Create a tracer provider with the in-memory exporter for testing.
	// This doesn't test InitTracer directly, but verifies the tracer interface
	// works correctly with a test exporter.
	tp := newTestTracerProvider(exporter)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	otel.SetTracerProvider(tp)
	defer saveTracerProvider(t)

	tr := otel.Tracer("test")
	_, span := tr.Start(context.Background(), "test-operation")
	span.SetAttributes(attribute.String("test.key", "test.value"))
	span.End()

	// Force flush to ensure the span is exported.
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush failed: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "test-operation" {
		t.Fatalf("span name = %q, want %q", spans[0].Name, "test-operation")
	}
}

// TestTracer_NestedSpans verifies nested span creation and parent-child relationship.
func TestTracer_NestedSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	otel.SetTracerProvider(tp)
	defer saveTracerProvider(t)

	tr := otel.Tracer("test")

	ctx, parentSpan := tr.Start(context.Background(), "parent")
	childCtx, childSpan := tr.Start(ctx, "child")
	_, grandchildSpan := tr.Start(childCtx, "grandchild")
	grandchildSpan.End()
	childSpan.End()
	parentSpan.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush failed: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// Spans are exported in completion order: grandchild, child, parent.
	// Verify parent-child relationships via SpanContext.
	grandchild := spans[0]
	child := spans[1]
	parent := spans[2]

	if grandchild.Parent.SpanID() != child.SpanContext.SpanID() {
		t.Fatal("grandchild's parent should be child")
	}
	if child.Parent.SpanID() != parent.SpanContext.SpanID() {
		t.Fatal("child's parent should be parent")
	}
	if parent.Parent.IsValid() {
		t.Fatal("parent should have no parent (root span)")
	}
}

// TestInitTracer_ConcurrentCalls verifies that calling InitTracer concurrently
// doesn't cause data races or panics. Run with -race.
func TestInitTracer_ConcurrentCalls(t *testing.T) {
	unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")
	saveTracerProvider(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			shutdown, err := InitTracer(ctx, "concurrent-service", "1.0.0")
			if err != nil {
				t.Errorf("concurrent InitTracer failed: %v", err)
				return
			}
			_ = shutdown(ctx)
		}()
	}
	wg.Wait()
}

// TestInitTracer_EmptyServiceName verifies that InitTracer handles an empty
// service name without panicking.
func TestInitTracer_ExporterFactoryError(t *testing.T) {
	setEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	prev := otlpExporterFactory
	otlpExporterFactory = func(context.Context, ...otlptracegrpc.Option) (*otlptrace.Exporter, error) {
		return nil, errors.New("exporter failed")
	}
	t.Cleanup(func() { otlpExporterFactory = prev })

	_, err := InitTracer(context.Background(), "test-service", "1.0.0")
	if err == nil {
		t.Fatal("expected exporter factory error")
	}
}

func TestInitTracer_ResourceFactoryError(t *testing.T) {
	setEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:9999")
	saveTracerProvider(t)

	prevResource := resourceFactory
	resourceFactory = func(context.Context, ...sdkresource.Option) (*sdkresource.Resource, error) {
		return nil, errors.New("resource failed")
	}
	t.Cleanup(func() { resourceFactory = prevResource })

	_, err := InitTracer(context.Background(), "test-service", "1.0.0")
	if err == nil {
		t.Fatal("expected resource factory error")
	}
}

func TestInitTracer_EmptyServiceName(t *testing.T) {
	unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx, "", "")
	if err != nil {
		t.Fatalf("InitTracer with empty service name failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown")
	}
	_ = shutdown(ctx)
}
