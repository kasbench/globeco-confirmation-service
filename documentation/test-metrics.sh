#!/bin/bash

# Test script to check if metrics are being exposed correctly and OpenTelemetry is working
echo "🔍 Testing OpenTelemetry Integration..."

# Get the pod name
POD_NAME=$(kubectl get pods -n globeco -l app=globeco-confirmation-service -o jsonpath='{.items[0].metadata.name}')

if [ -z "$POD_NAME" ]; then
    echo "❌ No pod found for globeco-confirmation-service"
    exit 1
fi

echo "📋 Found pod: $POD_NAME"

# Check if pod is running
POD_STATUS=$(kubectl get pod -n globeco $POD_NAME -o jsonpath='{.status.phase}')
echo "📊 Pod status: $POD_STATUS"

if [ "$POD_STATUS" != "Running" ]; then
    echo "❌ Pod is not running. Current status: $POD_STATUS"
    kubectl describe pod -n globeco $POD_NAME
    exit 1
fi

# Check recent logs for OpenTelemetry setup
echo ""
echo "📋 Checking recent logs for OpenTelemetry setup..."
RECENT_LOGS=$(kubectl logs -n globeco $POD_NAME --tail=20)
echo "$RECENT_LOGS"

# Check if we see trace JSON in logs (should NOT see this after fix)
TRACE_COUNT=$(echo "$RECENT_LOGS" | grep -c "SpanContext" || true)
if [ $TRACE_COUNT -gt 0 ]; then
    echo "⚠️  WARNING: Still seeing $TRACE_COUNT trace JSON entries in logs"
    echo "   This suggests traces are still being logged instead of sent to collector"
else
    echo "✅ No trace JSON in recent logs - traces likely being sent to collector"
fi

# Port forward to access metrics
echo ""
echo "🔗 Setting up port forward..."
kubectl port-forward -n globeco pod/$POD_NAME 8086:8086 &
PF_PID=$!

# Wait a moment for port forward to establish
sleep 3

# Test the metrics endpoint
echo "📊 Testing /metrics endpoint..."
METRICS_RESPONSE=$(curl -s http://localhost:8086/metrics)

if [ $? -eq 0 ] && [ ! -z "$METRICS_RESPONSE" ]; then
    echo "✅ Metrics endpoint is accessible"
    echo "📊 Total metrics lines: $(echo "$METRICS_RESPONSE" | wc -l)"
    
    # Check for specific confirmation service metrics
    echo ""
    echo "🔍 Confirmation service Prometheus metrics:"
    PROM_METRICS=$(echo "$METRICS_RESPONSE" | grep -E "confirmation_|messages_|api_calls_|kafka_" | head -5)
    if [ ! -z "$PROM_METRICS" ]; then
        echo "$PROM_METRICS"
        echo "✅ Prometheus metrics are being generated"
    else
        echo "⚠️  No confirmation service specific metrics found"
    fi
else
    echo "❌ Failed to access metrics endpoint"
fi

# Test health endpoints
echo ""
echo "🏥 Testing health endpoints..."
LIVE_RESPONSE=$(curl -s http://localhost:8086/health/live)
READY_RESPONSE=$(curl -s http://localhost:8086/health/ready)

if [[ $LIVE_RESPONSE == *"healthy"* ]]; then
    echo "✅ Live endpoint: OK"
else
    echo "❌ Live endpoint: $LIVE_RESPONSE"
fi

if [[ $READY_RESPONSE == *"healthy"* ]]; then
    echo "✅ Ready endpoint: OK"
else
    echo "❌ Ready endpoint: $READY_RESPONSE"
fi

# Clean up port forward
kill $PF_PID 2>/dev/null

# Check OpenTelemetry Collector logs
echo ""
echo "🔍 Checking OpenTelemetry Collector logs..."
OTEL_LOGS=$(kubectl logs -n monitoring -l app=otel-collector --tail=10 2>/dev/null || echo "Could not access collector logs")
if [[ $OTEL_LOGS == *"Could not access"* ]]; then
    echo "⚠️  Could not access OpenTelemetry Collector logs"
    echo "   Try: kubectl logs -n monitoring -l app=otel-collector"
else
    echo "📋 Recent collector activity:"
    echo "$OTEL_LOGS" | tail -5
fi

# Check environment variables
echo ""
echo "🔧 Checking OpenTelemetry environment variables..."
ENV_VARS=$(kubectl exec -n globeco $POD_NAME -- env | grep OTEL_ || echo "No OTEL_ environment variables found")
echo "$ENV_VARS"

echo ""
echo "✅ Test completed"
echo ""
echo "📋 Summary:"
echo "   - Pod Status: $POD_STATUS"
echo "   - Trace JSON in logs: $TRACE_COUNT entries"
echo "   - Metrics endpoint: $([ $? -eq 0 ] && echo "Working" || echo "Failed")"
echo ""
echo "🎯 Next steps:"
echo "   1. If trace JSON count > 0: Traces still being logged (check configuration)"
echo "   2. If trace JSON count = 0: Traces likely being sent to collector ✅"
echo "   3. Check Jaeger UI for traces: http://jaeger.orchestra.svc.cluster.local:16686"
echo "   4. Check Prometheus for metrics with service_name=confirmation-service"