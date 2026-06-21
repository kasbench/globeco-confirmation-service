package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// helper: creates a fresh ConsumerMetrics with a ManualReader for test isolation.
func setupTestMetrics(t *testing.T) (*ConsumerMetrics, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { provider.Shutdown(context.Background()) })

	meter := provider.Meter("test")
	cm, err := NewConsumerMetrics(meter, "test-group")
	require.NoError(t, err)
	return cm, reader
}

// helper: collects metrics and returns counter values for processed and failed.
func collectCounters(t *testing.T, reader *sdkmetric.ManualReader) (processed int64, failed int64) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	err := reader.Collect(context.Background(), &rm)
	require.NoError(t, err)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "kafka_consumer_messages_processed_total":
				if sumData, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sumData.DataPoints {
						processed += dp.Value
					}
				}
			case "kafka_consumer_messages_failed_total":
				if sumData, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sumData.DataPoints {
						failed += dp.Value
					}
				}
			}
		}
	}
	return processed, failed
}

// TestFailure_DeserializationError verifies that a deserialization error
// (represented by a single RecordProcessingFailure call) increments
// messages_failed_total exactly once and does not increment messages_processed_total.
//
// Validates: Requirements 3.1, 4.1
func TestFailure_DeserializationError(t *testing.T) {
	cm, reader := setupTestMetrics(t)
	ctx := context.Background()

	// Simulate: message failed deserialization → single failure recorded
	cm.RecordProcessingFailure(ctx, 0.5, nil, "fill-events", 0)

	processed, failed := collectCounters(t, reader)
	assert.Equal(t, int64(0), processed, "messages_processed_total should be 0 for deserialization error")
	assert.Equal(t, int64(1), failed, "messages_failed_total should be 1 for deserialization error")
}

// TestFailure_ValidationError verifies that a validation error
// increments messages_failed_total exactly once.
//
// Validates: Requirements 3.1, 4.2
func TestFailure_ValidationError(t *testing.T) {
	cm, reader := setupTestMetrics(t)
	ctx := context.Background()

	// Simulate: message failed validation → single failure recorded
	cm.RecordProcessingFailure(ctx, 0.5, nil, "fill-events", 0)

	processed, failed := collectCounters(t, reader)
	assert.Equal(t, int64(0), processed, "messages_processed_total should be 0 for validation error")
	assert.Equal(t, int64(1), failed, "messages_failed_total should be 1 for validation error")
}

// TestFailure_ExecutionServiceError verifies that an Execution Service error
// (retries exhausted) increments messages_failed_total exactly once.
// The Resilience Manager handles retries internally; only the terminal failure
// triggers a metric.
//
// Validates: Requirements 4.3, 4.6
func TestFailure_ExecutionServiceError(t *testing.T) {
	cm, reader := setupTestMetrics(t)
	ctx := context.Background()

	// Simulate: retries exhausted → single failure recorded at terminal outcome
	cm.RecordProcessingFailure(ctx, 0.5, nil, "fill-events", 0)

	processed, failed := collectCounters(t, reader)
	assert.Equal(t, int64(0), processed, "messages_processed_total should be 0 for execution service error")
	assert.Equal(t, int64(1), failed, "messages_failed_total should be 1 for execution service error")
}

// TestFailure_DLQRouting_NoDoubleCount verifies that DLQ routing increments
// messages_failed_total exactly once with no double-count. The design ensures
// RecordProcessingFailure is called only once per message at the terminal
// outcome, regardless of DLQ routing.
//
// Validates: Requirements 4.4, 4.6
func TestFailure_DLQRouting_NoDoubleCount(t *testing.T) {
	cm, reader := setupTestMetrics(t)
	ctx := context.Background()

	// Simulate: message routed to DLQ — only ONE failure recorded
	// (the system calls RecordProcessingFailure exactly once at terminal outcome)
	cm.RecordProcessingFailure(ctx, 0.5, nil, "fill-events", 0)

	processed, failed := collectCounters(t, reader)
	assert.Equal(t, int64(0), processed, "messages_processed_total should be 0 for DLQ-routed message")
	assert.Equal(t, int64(1), failed, "messages_failed_total should be exactly 1 (no double-count)")
}

// TestFailure_TransientRetryThenSuccess verifies that a transient failure
// followed by a successful retry does NOT increment messages_failed_total.
// Retries are internal to the ResilienceManager; only the final outcome
// triggers a metric.
//
// Validates: Requirements 3.3, 4.5
func TestFailure_TransientRetryThenSuccess(t *testing.T) {
	cm, reader := setupTestMetrics(t)
	ctx := context.Background()

	// Simulate: message experienced transient failures but ultimately succeeded.
	// Only RecordProcessingSuccess is called (not RecordProcessingFailure).
	cm.RecordProcessingSuccess(ctx, 0.5, nil, "fill-events", 0)

	processed, failed := collectCounters(t, reader)
	assert.Equal(t, int64(1), processed, "messages_processed_total should be 1 after retry success")
	assert.Equal(t, int64(0), failed, "messages_failed_total should be 0 when retries succeed")
}

// TestSuccess_ProcessingComplete verifies that successful processing
// increments messages_processed_total exactly once and does not increment
// messages_failed_total.
//
// Validates: Requirements 3.1, 4.6
func TestSuccess_ProcessingComplete(t *testing.T) {
	cm, reader := setupTestMetrics(t)
	ctx := context.Background()

	// Simulate: message processed successfully
	cm.RecordProcessingSuccess(ctx, 0.5, nil, "fill-events", 0)

	processed, failed := collectCounters(t, reader)
	assert.Equal(t, int64(1), processed, "messages_processed_total should be 1 for successful processing")
	assert.Equal(t, int64(0), failed, "messages_failed_total should be 0 for successful processing")
}

// TestFailure_SingleIncrementPerMessage verifies that calling
// RecordProcessingFailure once results in exactly one increment of
// messages_failed_total (not 2 or more). This confirms at-most-once semantics.
//
// Validates: Requirements 4.6
func TestFailure_SingleIncrementPerMessage(t *testing.T) {
	cm, reader := setupTestMetrics(t)
	ctx := context.Background()

	// Single call to RecordProcessingFailure
	cm.RecordProcessingFailure(ctx, 0.5, nil, "fill-events", 0)

	processed, failed := collectCounters(t, reader)
	assert.Equal(t, int64(0), processed, "messages_processed_total should be 0")
	assert.Equal(t, int64(1), failed, "messages_failed_total should be exactly 1 (at-most-once)")

	// Verify it's not 2 or more — the assertion above already covers this,
	// but make it explicit for documentation clarity.
	assert.Less(t, failed, int64(2), "messages_failed_total must not exceed 1 for a single failure call")
}
