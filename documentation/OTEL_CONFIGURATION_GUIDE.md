Here’s a step-by-step integration guide for another AI agent (or developer) to send telemetry data (metrics and traces) to the OpenTelemetry Collector as configured in your project. This guide is based on your current Kubernetes resource listing and service names.

---

# OpenTelemetry Collector Integration Guide

## Overview

Your Kubernetes cluster runs the following relevant services:
- **OpenTelemetry Collector** (`otel-collector-collector`): Receives traces and metrics via OTLP (gRPC/HTTP).
- **Jaeger** (`jaeger`): Receives traces for visualization.
- **Prometheus** (`prometheus-server`): Receives metrics via remote write.

---

## 1. Collector Endpoints

### a. OTLP Receiver (for traces and metrics)
The OpenTelemetry Collector exposes the OTLP receiver on the following ports:
- **gRPC:** 4317
- **HTTP:** 4318

**Kubernetes Service:**
- `otel-collector-collector` (ClusterIP, ports 4317/4318)

**Example Service DNS:**
- `otel-collector-collector.monitoring.svc.cluster.local:4317` (gRPC)
- `otel-collector-collector.monitoring.svc.cluster.local:4318` (HTTP)

---

## 2. How to Send Data

### a. Traces

**Send traces using OTLP (gRPC or HTTP) to:**
```
otel-collector-collector.monitoring.svc.cluster.local:4317
```

**Example (Python, OpenTelemetry SDK):**
```python
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.sdk.trace import TracerProvider

provider = TracerProvider()
processor = BatchSpanProcessor(
    OTLPSpanExporter(endpoint="otel-collector-collector.monitoring.svc.cluster.local:4317", insecure=True)
)
provider.add_span_processor(processor)
```
- Set `insecure=True` because the collector is configured with `tls.insecure: true`.

### b. Metrics

**Send metrics using OTLP (gRPC or HTTP) to the same endpoints as above.**

**Example (Python, OpenTelemetry SDK):**
```python
from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.metrics import MeterProvider

exporter = OTLPMetricExporter(endpoint="otel-collector-collector.monitoring.svc.cluster.local:4317", insecure=True)
reader = PeriodicExportingMetricReader(exporter)
provider = MeterProvider(metric_readers=[reader])
```

---

## 3. What Happens Next?

- **Traces** are forwarded to **Jaeger** at `jaeger.observability.svc.cluster.local:4317` (gRPC) and available in the Jaeger UI at `http://jaeger.orchestra.svc.cluster.local:16686`.
- **Metrics** are forwarded to **Prometheus** via remote write at `http://prometheus-server.monitor.svc.cluster.local:80/api/v1/write`.

---

## 4. Additional Notes

- **No authentication** is required for OTLP endpoints (insecure mode).
- **If your service is running in the same namespace (`default`)**, you can use the short DNS names.
- **If your service is running outside the cluster**, you must expose the collector service externally (e.g., via a LoadBalancer or Ingress).

---

## 5. Example Kubernetes Service Configuration

If you want to expose the collector to other namespaces or externally, you can patch the service:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: otel-collector-collector
  namespace: monitoring
spec:
  ports:
    - name: otlp-grpc
      port: 4317
      targetPort: 4317
    - name: otlp-http
      port: 4318
      targetPort: 4318
  type: ClusterIP # Change to LoadBalancer or NodePort if needed
```

---

## 6. Troubleshooting

- **Collector not receiving data?** Check pod logs for errors.
- **Traces not appearing in Jaeger?** Ensure the Jaeger service is reachable from the collector.
- **Metrics not in Prometheus?** Ensure the Prometheus remote write endpoint is correct and accessible.

---

## 7. References

- [OpenTelemetry Collector OTLP Exporter Docs](https://opentelemetry.io/docs/collector/configuration/#otlp)
- [OpenTelemetry Python SDK](https://opentelemetry.io/docs/instrumentation/python/exporters/)

---

If you need a sample manifest or code snippet for a specific language or framework, let me know!