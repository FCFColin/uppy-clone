// Package telemetry configures OpenTelemetry tracing for the server.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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

// otlpExporterFactory creates the OTLP exporter; tests may replace it.
var otlpExporterFactory = otlptracegrpc.New

// resourceFactory builds the OTel resource; tests may replace it.
var resourceFactory = sdkresource.New

func init() {
	tracer = otel.Tracer("github.com/uppy-clone/backend")
}

// Tracer returns the global tracer instance.
func Tracer() trace.Tracer {
	return tracer
}

// TracerConfig holds OpenTelemetry tracing pipeline configuration,
// passed explicitly by callers instead of read from environment variables.
type TracerConfig struct {
	Endpoint    string
	Insecure    bool
	SampleRatio float64
	Environment string
	Region      string
}

// InitTracer initializes the OpenTelemetry tracing pipeline.
// If cfg.Endpoint is empty, returns a no-op provider.
func InitTracer(ctx context.Context, serviceName, serviceVersion string, cfg TracerConfig) (func(context.Context) error, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		slog.Info("OpenTelemetry disabled: OTEL_EXPORTER_OTLP_ENDPOINT not set")
		return func(_ context.Context) error { return nil }, nil
	}

	exporter, err := func() (sdktrace.SpanExporter, error) {
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		return otlpExporterFactory(ctx, opts...)
	}()
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	env := cfg.Environment
	if env == "" {
		env = "development"
	}
	region := cfg.Region

	res, err := resourceFactory(ctx,
		sdkresource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			attribute.String("deployment.environment", env),
			attribute.String("cloud.region", region),
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
	sampleRatio := cfg.SampleRatio
	if sampleRatio == 0 {
		sampleRatio = 0.1
	}
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
