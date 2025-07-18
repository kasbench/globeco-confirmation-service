# Metrics Troubleshooting Guide

## Issue Summary
The confirmation service is generating traces (visible in logs) but metrics are not appearing in Prometheus via the OpenTelemetry Collector.

## Root Cause Analysis

### 1. Service Architecture
The service has **two separate metrics systems**:
- **Prometheus metrics** (pkg/metrics/metrics.go) - Used throughout the service ✅
- **OpenTelemetry metrics** (internal/utils/otel.go) - Configured but not used ❌

### 2. Issues Identified
1. **Missing Prometheus scraping configuration** - Service wasn't properly annotated for scraping
2. **Service port misconfiguration** - Service was exposing port 80 instead of 8086
3. **OpenTelemetry collector configuration** - May not be configured to scrape Prometheus metrics

## Solutions Applied

### 1. Fixed Kubernetes Configuration

#### Deployment (k8s/deployment.yaml)
- ✅ Added OpenTelemetry environment variables
- ✅ Added Prometheus scraping annotations to pod template

#### Service (k8s/service.yaml)  
- ✅ Fixed port configuration (8086 instead of 80)
- ✅ Added Prometheus scraping annotations

### 2. Environment Variables Added
```yaml
env:
  - name: OTEL_SERVICE_NAME
    value: "globeco-confirmation-service"
  - name: OTEL_SERVICE_VERSION
    value: "1.0.0"
  - name: OTEL_SERVICE_NAMESPACE
    value: "globeco"
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "otel-collector-collector.monitoring.svc.cluster.local:4317"
  - name: OTEL_EXPORTER_OTLP_PROTOCOL
    value: "grpc"
  - name: OTEL_RESOURCE_ATTRIBUTES
    value: "service.name=confirmation-service,service.version=1.0.0,service.namespace=globeco"
  - name: TRACING_ENABLED
    value: "true"
  - name: METRICS_ENABLED
    value: "true"
```

### 3. Prometheus Annotations Added
```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8086"
  prometheus.io/path: "/metrics"
```

## Verification Steps

### 1. Test Metrics Endpoint Directly
```bash
# Run the test script
./test-metrics.sh
```

### 2. Check Pod Status
```bash
kubectl get pods -n globeco -l app=globeco-confirmation-service
kubectl logs -n globeco -l app=globeco-confirmation-service --tail=50
```

### 3. Verify Service Configuration
```bash
kubectl get svc -n globeco globeco-confirmation-service -o yaml
```

### 4. Test Metrics Endpoint Manually
```bash
# Port forward to the service
kubectl port-forward -n globeco svc/globeco-confirmation-service 8086:8086

# In another terminal, test the endpoint
curl http://localhost:8086/metrics
```

## Expected Metrics

The service should expose these metrics:

### Message Processing
- `confirmation_messages_processed_total`
- `confirmation_messages_failed_total`
- `confirmation_message_processing_duration_seconds`
- `confirmation_messages_processing_current`

### API Calls
- `confirmation_api_calls_total{method, endpoint, status_code}`
- `confirmation_api_call_duration_seconds{method, endpoint}`
- `confirmation_api_calls_in_flight`

### Kafka
- `confirmation_kafka_messages_consumed_total`
- `confirmation_kafka_consumer_lag`
- `confirmation_kafka_connection_errors_total`

### System
- `confirmation_goroutines_active`
- `confirmation_memory_usage_bytes`
- `confirmation_cpu_usage_percent`

## OpenTelemetry Collector Configuration

If metrics still don't appear, check your OpenTelemetry Collector configuration:

### Required Receivers
```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: 'confirmation-service'
          kubernetes_sd_configs:
            - role: pod
              namespaces:
                names:
                  - globeco
          relabel_configs:
            - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
              action: keep
              regex: true
            - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
              action: replace
              target_label: __metrics_path__
              regex: (.+)
            - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
              action: replace
              regex: ([^:]+)(?::\d+)?;(\d+)
              replacement: $1:$2
              target_label: __address__
```

### Required Exporters
```yaml
exporters:
  prometheus:
    endpoint: "0.0.0.0:8889"
```

### Pipeline Configuration
```yaml
service:
  pipelines:
    metrics:
      receivers: [prometheus]
      exporters: [prometheus]
```

## Next Steps

1. **Deploy the updated configuration**:
   ```bash
   kubectl apply -f k8s/
   ```

2. **Wait for pod restart and test**:
   ```bash
   kubectl rollout status deployment/globeco-confirmation-service -n globeco
   ./test-metrics.sh
   ```

3. **Check OpenTelemetry Collector logs**:
   ```bash
   kubectl logs -n monitoring -l app=otel-collector
   ```

4. **Verify Prometheus targets**:
   - Access Prometheus UI
   - Go to Status > Targets
   - Look for confirmation-service targets

## Alternative: Direct Prometheus Scraping

If the OpenTelemetry Collector approach doesn't work, you can configure Prometheus to scrape directly:

```yaml
# Add to Prometheus configuration
scrape_configs:
  - job_name: 'confirmation-service'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - globeco
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target_label: __address__
```