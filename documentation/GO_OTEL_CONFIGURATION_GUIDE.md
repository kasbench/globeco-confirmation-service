# Go OpenTelemetry Instrumentation Guide for GlobeCo Microservices

This guide describes the **standard, consistent way** to instrument any Go microservice in the GlobeCo suite for metrics and distributed tracing. Follow these steps exactly to ensure all services are observable in the same way, making maintenance and debugging easier.

---

## 1. Add Required Dependencies

Add the following dependencies to your `go.mod`:

```bash
go get go.opentelemetry.io/otel/sdk@v1.22.0
# For OTLP exporter
 go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@v1.22.0
 go get go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc@v1.22.0
# For resource attributes
 go get go.opentelemetry.io/otel/sdk/resource@v1.22.0
# For Prometheus metrics (optional, if you want direct Prometheus scrape)
 go get go.opentelemetry.io/contrib/exporters/metric/prometheus@v0.49.0
# For zap logging integration (optional, for context propagation)
 go get go.opentelemetry.io/contrib/instrumentation/go.uber.org/zap/otelzap@v0.49.0
```

---

## 2. Configure Telemetry in Your Application

Set up your OpenTelemetry pipeline at application startup. Use the following configuration, replacing only the service name/version/namespace as appropriate for your service; all other settings should remain identical for consistency.

### Example Setup (main.go)

```go
import (
    "context"
    "os"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
    "go.opentelemetry.io/otel/sdk/resource"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
    "go.opentelemetry.io/otel/sdk/trace"
    "go.opentelemetry.io/otel/sdk/metric"
    "google.golang.org/grpc"
)

func setupOTel(ctx context.Context) (func(context.Context) error, error) {
    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceNameKey.String("YOUR-SERVICE-NAME"),
            semconv.ServiceVersionKey.String("1.0.0"),
            semconv.ServiceNamespaceKey.String("globeco"),
        ),
    )
    if err != nil {
        return nil, err
    }

    // Traces exporter
    traceExp, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint("otel-collector-collector.monitoring.svc.cluster.local:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }
    tracerProvider := trace.NewTracerProvider(
        trace.WithBatcher(traceExp),
        trace.WithResource(res),
    )
    otel.SetTracerProvider(tracerProvider)

    // Metrics exporter
    metricExp, err := otlpmetricgrpc.New(ctx,
        otlpmetricgrpc.WithEndpoint("otel-collector-collector.monitoring.svc.cluster.local:4317"),
        otlpmetricgrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }
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
```

- **Service name/version/namespace**: Set these to identify your service in observability tools.
- **Endpoint**: Always use `otel-collector-collector.monitoring.svc.cluster.local:4317` (gRPC, insecure).
- **Shutdown**: Call the returned shutdown function on application exit for graceful shutdown.

#### Environment Variables (Recommended for Consistency)

You may also configure service name/version/namespace and collector endpoint via environment variables for 12-factor compliance:

- `OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector-collector.monitoring.svc.cluster.local:4317`
- `OTEL_SERVICE_NAME=YOUR-SERVICE-NAME`
- `OTEL_SERVICE_VERSION=1.0.0`
- `OTEL_SERVICE_NAMESPACE=globeco`

---

## 3. What Gets Instrumented by Default?

- **Metrics:**
  - Go runtime metrics (GC, memory, goroutines, etc.)
  - HTTP server metrics (if using instrumented router/middleware, e.g., go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp)
  - Custom metrics via OpenTelemetry API
- **Traces:**
  - All HTTP requests (if using otelhttp middleware)
  - Spans for outgoing HTTP/gRPC requests (if instrumented)
  - Custom spans in business logic (see below)

---

## 4. How to View Telemetry Data

- **Metrics:**
  - Collected by the OpenTelemetry Collector and forwarded to Prometheus.
  - View in Prometheus or Grafana dashboards.
- **Traces:**
  - Collected by the OpenTelemetry Collector and forwarded to Jaeger.
  - View in Jaeger UI (e.g., `http://jaeger.orchestra.svc.cluster.local:16686`).

---

## 5. How to Add Custom Spans and Metrics (Optional)

### Custom Spans

```go
tr := otel.Tracer("YOUR-SERVICE-NAME")
ctx, span := tr.Start(ctx, "operation-name")
defer span.End()
// ... your business logic ...
```

### Custom Metrics

```go
meter := otel.Meter("YOUR-SERVICE-NAME")
counter, _ := meter.Int64Counter("my_custom_counter")
counter.Add(ctx, 1)
```

---

## 6. Example: Consistent Configuration for a Service

**go.mod**
```go
require (
    go.opentelemetry.io/otel/sdk v1.22.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.22.0
    go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.22.0
    go.opentelemetry.io/otel/sdk/resource v1.22.0
)
```

**main.go**
```go
// ... see setupOTel function above ...
```

**Environment variables**
```env
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector-collector.monitoring.svc.cluster.local:4317
OTEL_SERVICE_NAME=globeco-portfolio-accounting-service
OTEL_SERVICE_VERSION=1.0.0
OTEL_SERVICE_NAMESPACE=globeco
```

---

## 7. Verification Checklist

- [x] **Dependencies** in `go.mod` match this guide exactly.
- [x] **Service name/version/namespace** are set for both metrics and traces.
- [x] **Endpoints** for metrics and traces use the OTLP gRPC endpoint: `otel-collector-collector.monitoring.svc.cluster.local:4317`.
- [x] **Resource attributes** are set for service name, version, and namespace.
- [x] **Shutdown** function is called on application exit.
- [x] **otelhttp** middleware is used for HTTP handlers (if applicable).

---

## 8. References
- See `documentation/OTEL_CONFIGURATION_GUIDE.md` for OpenTelemetry Collector setup and troubleshooting.
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/instrumentation/go/)
- [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
- [Jaeger](https://www.jaegertracing.io/)
- [Prometheus](https://prometheus.io/)

---

**By following this guide, every Go microservice in the GlobeCo suite will be instrumented in a consistent, maintainable, and debuggable way.**
