package utils

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TracingConfig represents tracing configuration
type TracingConfig struct {
	Enabled        bool
	ServiceName    string
	ServiceVersion string
	Exporter       string // stdout, jaeger, otlp
	OTLPEndpoint   string
}

// TracingProvider wraps the OpenTelemetry tracer provider
type TracingProvider struct {
	provider *trace.TracerProvider
	tracer   oteltrace.Tracer
}

// NewTracingProvider creates a new tracing provider
func NewTracingProvider(config TracingConfig) (*TracingProvider, error) {
	if !config.Enabled {
		return &TracingProvider{}, nil
	}

	// Create exporter based on configuration
	var exporter trace.SpanExporter
	var err error

	switch config.Exporter {
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}
	case "jaeger":
		// TODO: Implement Jaeger exporter when needed
		return nil, fmt.Errorf("jaeger exporter not implemented yet")
	case "otlp":
		exporter, err = otlptracegrpc.New(context.Background(),
			otlptracegrpc.WithEndpoint(config.OTLPEndpoint),
			otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported exporter: %s", config.Exporter)
	}

	// Create tracer provider
	provider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(createResource(config.ServiceName, config.ServiceVersion)),
	)

	// Set global tracer provider
	otel.SetTracerProvider(provider)

	// Create tracer
	tracer := provider.Tracer(config.ServiceName)

	return &TracingProvider{
		provider: provider,
		tracer:   tracer,
	}, nil
}

// createResource creates an OpenTelemetry resource
func createResource(serviceName, serviceVersion string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
		semconv.ServiceNamespace("globeco"),
	)
}

// Shutdown shuts down the tracing provider
func (tp *TracingProvider) Shutdown(ctx context.Context) error {
	if tp.provider != nil {
		return tp.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span with the given name
func (tp *TracingProvider) StartSpan(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	if tp.tracer == nil {
		// Return a no-op span if tracing is disabled
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	return tp.tracer.Start(ctx, spanName, opts...)
}

// StartKafkaConsumerSpan starts a span for Kafka message consumption
func (tp *TracingProvider) StartKafkaConsumerSpan(ctx context.Context, topic string, partition int, offset int64) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("kafka.consume %s", topic)
	ctx, span := tp.StartSpan(ctx, spanName)

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination", topic),
			attribute.String("messaging.operation", "receive"),
			attribute.Int("messaging.kafka.partition", partition),
			attribute.Int64("messaging.kafka.offset", offset),
		)
	}

	return ctx, span
}

// StartHTTPClientSpan starts a span for HTTP client calls
func (tp *TracingProvider) StartHTTPClientSpan(ctx context.Context, method, url string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("HTTP %s", method)
	ctx, span := tp.StartSpan(ctx, spanName)

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("http.method", method),
			attribute.String("http.url", url),
			attribute.String("span.kind", "client"),
		)
	}

	return ctx, span
}

// AddSpanAttributes adds attributes to the current span
func (tp *TracingProvider) AddSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := oteltrace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// AddSpanEvent adds an event to the current span
func (tp *TracingProvider) AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := oteltrace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, oteltrace.WithAttributes(attrs...))
	}
}

// SetSpanError sets an error on the current span
func (tp *TracingProvider) SetSpanError(ctx context.Context, err error) {
	span := oteltrace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSpanStatus sets the status of the current span
func (tp *TracingProvider) SetSpanStatus(ctx context.Context, code codes.Code, description string) {
	span := oteltrace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetStatus(code, description)
	}
}
