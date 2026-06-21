package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	promClient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// TestMetricsEndpoint_AllMetricsExposed verifies that all 8 OTel-registered
// metrics appear in Prometheus text format on the /metrics endpoint with correct
// suffixes: _total for counters, _bucket/_count/_sum for histograms.
// It also verifies HTTP 200 status and correct Content-Type header.
//
// **Validates: Requirements 11.1, 11.2, 11.3, 11.4, 11.5, 11.7, 15.8**
func TestMetricsEndpoint_AllMetricsExposed(t *testing.T) {
	// Use a custom Prometheus registry for test isolation (avoids duplicate
	// registration errors when running alongside other tests in the same process).
	registry := promClient.NewRegistry()

	// Create the OTel Prometheus exporter with the custom registry.
	exporter, err := prometheus.New(
		prometheus.WithRegisterer(registry),
	)
	require.NoError(t, err, "failed to create Prometheus exporter")

	// Create a MeterProvider with the Prometheus exporter as reader.
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	defer func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	}()

	// Create ConsumerMetrics from the MeterProvider.
	meter := provider.Meter("globeco-confirmation-service")
	cm, err := NewConsumerMetrics(meter, "test-consumer-group")
	require.NoError(t, err, "failed to create ConsumerMetrics")

	ctx := context.Background()

	// Trigger at least one successful message processing cycle.
	latencySuccess := 0.125
	cm.RecordPollSuccess(ctx, 0.05, "fills", 0)
	cm.RecordProcessingSuccess(ctx, 0.200, &latencySuccess, "fills", 0)

	// Trigger at least one failed message processing cycle.
	latencyFailure := 0.350
	cm.RecordPollSuccess(ctx, 0.03, "fills", 1)
	cm.RecordProcessingFailure(ctx, 0.150, &latencyFailure, "fills", 1)

	// Set up an HTTP test server using promhttp.HandlerFor with the custom registry.
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make an HTTP GET request to the /metrics endpoint.
	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err, "failed to GET /metrics endpoint")
	defer resp.Body.Close()

	// Verify HTTP 200 status.
	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected HTTP 200 from /metrics")

	// Verify Content-Type contains expected value.
	contentType := resp.Header.Get("Content-Type")
	assert.True(t,
		strings.Contains(contentType, "text/plain") || strings.Contains(contentType, "text/openmetrics"),
		"Content-Type should contain 'text/plain' or 'text/openmetrics', got: %s", contentType,
	)

	// Read the response body.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read /metrics response body")
	metricsOutput := string(body)

	// --- Verify counters appear with _total suffix ---
	counterMetrics := []string{
		"kafka_consumer_messages_processed_total",
		"kafka_consumer_messages_failed_total",
		"kafka_consumer_processing_seconds_total",
		"kafka_consumer_idle_seconds_total",
		"kafka_consumer_records_polled_total",
		"kafka_consumer_poll_seconds_total",
	}

	for _, metricName := range counterMetrics {
		assert.Contains(t, metricsOutput, metricName,
			"counter metric %q should be present in /metrics output", metricName)
	}

	// --- Verify histograms appear with _bucket, _count, _sum suffixes ---
	histogramMetrics := []struct {
		baseName string
	}{
		{baseName: "kafka_consumer_processing_duration_seconds"},
		{baseName: "kafka_consumer_message_latency_seconds"},
	}

	for _, hm := range histogramMetrics {
		bucketName := hm.baseName + "_bucket"
		countName := hm.baseName + "_count"
		sumName := hm.baseName + "_sum"

		assert.Contains(t, metricsOutput, bucketName,
			"histogram %q should have _bucket series", hm.baseName)
		assert.Contains(t, metricsOutput, countName,
			"histogram %q should have _count series", hm.baseName)
		assert.Contains(t, metricsOutput, sumName,
			"histogram %q should have _sum series", hm.baseName)
	}
}

// TestMetricsEndpoint_ExistingPrometheusMetricsCoexist verifies that existing
// direct-registered Prometheus metrics remain present and unchanged when OTel
// metrics are added via the Prometheus exporter.
//
// **Validates: Requirements 11.2, 11.5**
func TestMetricsEndpoint_ExistingPrometheusMetricsCoexist(t *testing.T) {
	// Use a custom Prometheus registry for test isolation.
	registry := promClient.NewRegistry()

	// Register a direct Prometheus counter (simulating an existing metric).
	existingCounter := promClient.NewCounter(promClient.CounterOpts{
		Name: "existing_direct_metric_total",
		Help: "A pre-existing direct-registered Prometheus metric",
	})
	require.NoError(t, registry.Register(existingCounter))
	existingCounter.Add(42)

	// Create OTel Prometheus exporter with the same registry.
	exporter, err := prometheus.New(
		prometheus.WithRegisterer(registry),
	)
	require.NoError(t, err, "failed to create Prometheus exporter")

	// Create MeterProvider and ConsumerMetrics.
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	defer func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	}()

	meter := provider.Meter("globeco-confirmation-service")
	cm, err := NewConsumerMetrics(meter, "test-group")
	require.NoError(t, err)

	ctx := context.Background()
	cm.RecordPollSuccess(ctx, 0.1, "fills", 0)

	// Set up test server using a gatherer that combines the registry.
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	metricsOutput := string(body)

	// Verify the existing direct-registered metric is still present.
	assert.Contains(t, metricsOutput, "existing_direct_metric_total",
		"existing direct-registered Prometheus metric should remain present")
	assert.Contains(t, metricsOutput, "42",
		"existing direct-registered Prometheus metric should have correct value")

	// Verify OTel metrics are also present.
	assert.Contains(t, metricsOutput, "kafka_consumer_records_polled_total",
		"OTel metric should coexist with direct-registered metrics")
}

// TestMetricsEndpoint_ContentTypeAndStatus verifies that the /metrics endpoint
// returns HTTP 200 with proper Content-Type even when no metrics have been recorded.
//
// **Validates: Requirements 11.3**
func TestMetricsEndpoint_ContentTypeAndStatus(t *testing.T) {
	registry := promClient.NewRegistry()

	exporter, err := prometheus.New(
		prometheus.WithRegisterer(registry),
	)
	require.NoError(t, err)

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	defer func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	}()

	// Create metrics but don't record anything — endpoint should still work.
	meter := provider.Meter("globeco-confirmation-service")
	_, err = NewConsumerMetrics(meter, "test-group")
	require.NoError(t, err)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	contentType := resp.Header.Get("Content-Type")
	assert.True(t,
		strings.Contains(contentType, "text/plain") || strings.Contains(contentType, "text/openmetrics"),
		"Content-Type should be text/plain or text/openmetrics, got: %s", contentType,
	)
}
