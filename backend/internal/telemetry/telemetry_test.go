package telemetry

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

// tracerConfigFromEnv builds a TracerConfig from environment variables.
// Keeps existing env-based test setup working with the new cfg-based InitTracer API.
// Inlines the deleted isOTLPInsecure()/getSampleRatio() helpers.
func tracerConfigFromEnv() TracerConfig {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	ratio := 0.1
	if s := os.Getenv("OTEL_SAMPLE_RATIO"); s != "" {
		if r, err := strconv.ParseFloat(s, 64); err == nil && r >= 0.0 && r <= 1.0 {
			ratio = r
		}
	}
	return TracerConfig{
		Endpoint:    endpoint,
		Insecure:    strings.HasPrefix(endpoint, "http://"),
		SampleRatio: ratio,
	}
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
// TestInitTracer_NoEndpoint_ReturnsNoop verifies that when
// OTEL_EXPORTER_OTLP_ENDPOINT is not set, InitTracer returns a noop
// shutdown function and no error.
func TestInitTracer_NoEndpoint_ReturnsNoop(t *testing.T) {
	unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")
	unsetEnv(t, "OTEL_SAMPLE_RATIO")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx, "test-service", "1.0.0", tracerConfigFromEnv())
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
	shutdown, err := InitTracer(ctx, "test-service", "1.0.0", tracerConfigFromEnv())
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

	shutdown, err := InitTracer(ctx, "test-service", "1.0.0", tracerConfigFromEnv())
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

	shutdown, err := InitTracer(ctx, "test-service", "1.0.0", tracerConfigFromEnv())
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

	shutdown, err := InitTracer(ctx, "test-service", "1.0.0", tracerConfigFromEnv())
	// With no endpoint, the context is not used, so this should succeed.
	if err != nil {
		t.Fatalf("InitTracer with cancelled context and no endpoint failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown")
	}
	_ = shutdown(context.Background())
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
			shutdown, err := InitTracer(ctx, "concurrent-service", "1.0.0", tracerConfigFromEnv())
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

	_, err := InitTracer(context.Background(), "test-service", "1.0.0", tracerConfigFromEnv())
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

	_, err := InitTracer(context.Background(), "test-service", "1.0.0", tracerConfigFromEnv())
	if err == nil {
		t.Fatal("expected resource factory error")
	}
}

func TestInitTracer_EmptyServiceName(t *testing.T) {
	unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx, "", "", tracerConfigFromEnv())
	if err != nil {
		t.Fatalf("InitTracer with empty service name failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown")
	}
	_ = shutdown(ctx)
}
