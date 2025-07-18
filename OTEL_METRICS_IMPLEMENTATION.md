# OpenTelemetry Metrics Implementation

## Overview

This document describes the implementation of OpenTelemetry metrics alongside the existing Prometheus metrics in the confirmation service. This hybrid approach ensures metrics are sent to the OpenTelemetry Collector while maintaining backward compatibility.

## What Was Implemented

### 1. OpenTelemetry Metrics Package (`pkg/otelmetrics/`)

Created a new OpenTelemetry-based metrics implementation that mirrors the existing Prometheus metrics:

#### Key Features:
- **Full OpenTelemetry SDK integration** - Uses the official OpenTelemetry Go SDK
- **Context-aware metrics** - All metrics operations accept context for proper tracing correlation
- **Comprehensive metric types** - Counters, Histograms, Gauges, and UpDownCounters
- **Automatic system metrics** - Runtime metrics (goroutines, memory, CPU)

#### Metrics Implemented:
- **Message Processing**: `messages_processed_total`, `messages_failed_total`, `message_processing_duration_seconds`
- **API Calls**: `api_calls_total`, `api_call_duration_seconds`, `api_calls_in_flight`
- **Kafka**: `kafka_messages_consumed_total`, `kafka_consumer_lag`, `kafka_connection_errors_total`
- **Circuit Breaker**: `circuit_breaker_state`, `circuit_breaker_operations_total`
- **Health Checks**: `health_check_status`, `health_check_duration_seconds`
- **System**: `goroutines_active`, `memory_usage_bytes`, `cpu_usage_percent`

### 2. Updated OpenTelemetry Configuration (`internal/utils/otel.go`)

Fixed the OpenTelemetry configuration to match GlobeCo standards:
- ✅ Changed from `WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials()))` to `WithInsecure()`
- ✅ Proper OTLP endpoint configuration
- ✅ Consistent with documentation standards

### 3. Kubernetes Configuration Updates

#### Deployment (`k8s/deployment.yaml`):
- ✅ Added OpenTelemetry environment variables
- ✅ Added Prometheus scraping annotations
- ✅ Proper service configuration

#### Service (`k8s/service.yaml`):
- ✅ Fixed port configuration (8086)
- ✅ Added Prometheus scraping annotations

### 4. Hybrid Metrics Approach

The service now runs **both** metrics systems in parallel:

1. **Prometheus Metrics** (existing) - Continue to work as before, served at `/metrics`
2. **OpenTelemetry Metrics** (new) - Sent to OTLP collector via gRPC

This ensures:
- ✅ **No breaking changes** - Existing Prometheus scraping continues to work
- ✅ **OpenTelemetry integration** - Metrics are sent to the collector
- ✅ **Gradual migration path** - Can transition fully to OpenTelemetry later

## Architecture

```
┌─────────────────────┐    ┌──────────────────────┐    ┌─────────────────────┐
│   Application       │    │  OpenTelemetry       │    │   Prometheus        │
│   Code              │    │  Collector           │    │   (via /metrics)    │
│                     │    │                      │    │                     │
│  ┌─────────────┐    │    │  ┌─────────────────┐ │    │  ┌─────────────────┐│
│  │ Prometheus  │────┼────┼──│ Prometheus      │ │    │  │ Prometheus      ││
│  │ Metrics     │    │    │  │ Scraper         │ │    │  │ Server          ││
│  └─────────────┘    │    │  └─────────────────┘ │    │  └─────────────────┘│
│                     │    │                      │    │                     │
│  ┌─────────────┐    │    │  ┌─────────────────┐ │    │  ┌─────────────────┐│
│  │ OpenTelemetry│────┼────┼──│ OTLP Receiver   │ │    │  │ Remote Write    ││
│  │ Metrics     │    │    │  │ (gRPC:4317)     │ │    │  │ Endpoint        ││
│  └─────────────┘    │    │  └─────────────────┘ │    │  └─────────────────┘│
└─────────────────────┘    └──────────────────────┘    └─────────────────────┘
```

## Deployment Instructions

### 1. Deploy Updated Configuration

```bash
# Apply the updated Kubernetes manifests
kubectl apply -f k8s/

# Wait for rollout to complete
kubectl rollout status deployment/globeco-confirmation-service -n globeco
```

### 2. Verify Metrics Are Working

#### Test Prometheus Metrics (existing):
```bash
# Port forward to the service
kubectl port-forward -n globeco svc/globeco-confirmation-service 8086:8086

# Test metrics endpoint
curl http://localhost:8086/metrics | grep confirmation_
```

#### Test OpenTelemetry Metrics (new):
```bash
# Check OpenTelemetry Collector logs
kubectl logs -n monitoring -l app=otel-collector --tail=50

# Look for OTLP receiver activity and metric processing
```

### 3. Verify in Prometheus

1. Access Prometheus UI
2. Go to **Status > Targets**
3. Look for `confirmation-service` targets
4. Query for metrics: `{__name__=~"confirmation_.*"}`

## Expected Metrics in Prometheus

After deployment, you should see these metrics:

### From Prometheus Scraping (`/metrics` endpoint):
```
confirmation_messages_processed_total
confirmation_messages_failed_total
confirmation_message_processing_duration_seconds
confirmation_api_calls_total
confirmation_kafka_messages_consumed_total
# ... and more
```

### From OpenTelemetry Collector:
```
messages_processed_total{service_name="confirmation-service"}
messages_failed_total{service_name="confirmation-service"}
message_processing_duration_seconds{service_name="confirmation-service"}
api_calls_total{service_name="confirmation-service"}
# ... and more
```

## Monitoring and Troubleshooting

### 1. Check Service Logs
```bash
kubectl logs -n globeco -l app=globeco-confirmation-service --tail=100
```

### 2. Check OpenTelemetry Collector
```bash
kubectl logs -n monitoring -l app=otel-collector --tail=100
```

### 3. Test Metrics Endpoint
```bash
./test-metrics.sh
```

### 4. Verify Environment Variables
```bash
kubectl get pod -n globeco -l app=globeco-confirmation-service -o yaml | grep -A 20 env:
```

## Benefits of This Implementation

1. **Immediate Metrics Visibility** - OpenTelemetry metrics are now being generated and sent to the collector
2. **No Service Disruption** - Existing Prometheus metrics continue to work
3. **Standards Compliance** - Follows GlobeCo OpenTelemetry standards
4. **Rich Context** - OpenTelemetry metrics include proper service attributes and context
5. **Future-Proof** - Easy migration path to pure OpenTelemetry metrics

## Next Steps

1. **Monitor for 24-48 hours** to ensure stability
2. **Verify metrics appear in Prometheus** via the OpenTelemetry Collector
3. **Create Grafana dashboards** using the new OpenTelemetry metrics
4. **Consider migrating other services** using the same pattern

## Rollback Plan

If issues occur, you can quickly rollback:

```bash
# Remove OpenTelemetry environment variables from deployment
kubectl patch deployment globeco-confirmation-service -n globeco --type='json' \
  -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/env"}]'

# The service will continue working with Prometheus metrics only
```

The service is designed to gracefully handle OpenTelemetry failures and will continue operating normally with just Prometheus metrics if needed.