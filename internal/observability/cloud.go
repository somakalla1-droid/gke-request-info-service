package observability

import (
	"context"
	"fmt"

	"cloud.google.com/go/profiler"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

type Config struct {
	ProjectID      string
	ServiceName    string
	ServiceVersion string
	ClusterName    string
	Region         string
}

// StartTracing configures authenticated OTLP export to Google Cloud's
// Telemetry API. The returned function flushes buffered spans during a
// graceful application shutdown.
func StartTracing(ctx context.Context, config Config) (func(context.Context) error, error) {
	credentials, err := oauth.NewApplicationDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("load application default credentials: %w", err)
	}

	res, err := resource.New(
		ctx,
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", config.ServiceName),
			attribute.String("service.version", config.ServiceVersion),
			attribute.String("gcp.project_id", config.ProjectID),
			attribute.String("cloud.provider", "gcp"),
			attribute.String("cloud.region", config.Region),
			attribute.String("k8s.cluster.name", config.ClusterName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithEndpoint("telemetry.googleapis.com:443"),
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(credentials)),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}

// StartProfiler is independent from tracing so a backend initialization
// failure does not stop HTTP request handling.
func StartProfiler(config Config) error {
	if err := profiler.Start(profiler.Config{
		ProjectID:      config.ProjectID,
		Service:        config.ServiceName,
		ServiceVersion: config.ServiceVersion,
	}); err != nil {
		return fmt.Errorf("start Cloud Profiler: %w", err)
	}
	return nil
}
