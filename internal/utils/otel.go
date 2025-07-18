package utils

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// OTelConfig represents OpenTelemetry configuration
type OTelConfig struct {
	ServiceName      string
	ServiceVersion   string
	ServiceNamespace string
	OTLPEndpoint     string
	Enabled          bool
}

// SetupOTel configures OpenTelemetry following GlobeCo standards
func SetupOTel(ctx context.Context, config OTelConfig) (func(context.Context) error, error) {
	if !config.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	// Use environment variables if available, otherwise use config values
	serviceName := getEnvOrDefault("OTEL_SERVICE_NAME", config.ServiceName)
	serviceVersion := getEnvOrDefault("OTEL_SERVICE_VERSION", config.ServiceVersion)
	serviceNamespace := getEnvOrDefault("OTEL_SERVICE_NAMESPACE", config.ServiceNamespace)
	otlpEndpoint := getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", config.OTLPEndpoint)

	// Create resource with service attributes
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			semconv.ServiceNamespaceKey.String(serviceNamespace),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Setup traces exporter
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create tracer provider
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExp),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	// Setup metrics exporter
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(otlpEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	// Create meter provider
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExp)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	// Return shutdown function
	return func(ctx context.Context) error {
		err1 := tracerProvider.Shutdown(ctx)
		err2 := meterProvider.Shutdown(ctx)
		if err1 != nil {
			return err1
		}
		return err2
	}, nil
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}