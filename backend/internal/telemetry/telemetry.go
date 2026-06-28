// Package telemetry configures OpenTelemetry tracing for the server.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Enterprise rationale: Distributed tracing is essential for debugging
// latency in microservices. OpenTelemetry is the CNCF graduated standard,
// replacing vendor-specific solutions (Jaeger tracing, Zipkin).
// Trace ID correlation across services enables root-cause analysis of
// P99 latency spikes. Trade-off: Tracing adds ~1% overhead per request
// and requires a collector backend (Jaeger/Tempo).

var tracer trace.Tracer

func init() {
	tracer = otel.Tracer("github.com/uppy-clone/backend")
}

// Tracer returns the global tracer instance.
func Tracer() trace.Tracer {
	return tracer
}

// InitTracer initializes the OpenTelemetry tracing pipeline.
// If OTEL_EXPORTER_OTLP_ENDPOINT is not set, returns a no-op provider.
func InitTracer(ctx context.Context, serviceName, serviceVersion string) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		slog.Info("OpenTelemetry disabled: OTEL_EXPORTER_OTLP_ENDPOINT not set")
		return func(_ context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	// Enterprise rationale: Default sampler is ParentBased(AlwaysSample) which
	// captures 100% of traces. At high QPS this overloads the OTLP collector
	// and storage. ParentBased(TraceIDRatioBased(ratio)) honors upstream
	// sampling decisions while applying a head-based ratio for root spans.
	// OTEL_SAMPLE_RATIO controls the ratio (0.0-1.0), default 0.1 (10%).
	sampleRatio := getSampleRatio()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(sampleRatio),
		)),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OpenTelemetry enabled", "endpoint", endpoint, "sample_ratio", sampleRatio)
	return provider.Shutdown, nil
}

// getSampleRatio reads OTEL_SAMPLE_RATIO from env (0.0-1.0), default 0.1.
func getSampleRatio() float64 {
	if v := os.Getenv("OTEL_SAMPLE_RATIO"); v != "" {
		if r, err := strconv.ParseFloat(v, 64); err == nil && r >= 0 && r <= 1 {
			return r
		}
	}
	return 0.1
}
