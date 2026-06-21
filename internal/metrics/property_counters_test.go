package metrics

import (
	"context"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestPropertySuccessFailureMutualExclusivity validates Property 2:
// Success and failure counters are mutually exclusive and exhaustive.
//
// **Validates: Requirements 3.1, 3.2, 3.4, 3.5, 4.1, 4.2, 4.6**
//
// For any Kafka fill message that reaches a terminal outcome, exactly one of
// messages_processed_total (on success) or messages_failed_total (on failure)
// is incremented by 1, and the other is NOT incremented. For FetchMessage errors
// or cancellations, neither counter is incremented.
func TestPropertySuccessFailureMutualExclusivity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// outcome: 0 = success, 1 = failure, 2 = cancellation (no terminal outcome)
	properties.Property("exactly one counter incremented per terminal outcome, never both and never neither", prop.ForAll(
		func(outcome int, topic string, partition int, duration float64) bool {
			// Create a fresh MeterProvider with a ManualReader for each test iteration
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer provider.Shutdown(context.Background())

			meter := provider.Meter("test")
			cm, err := NewConsumerMetrics(meter, "test-group")
			if err != nil {
				t.Logf("failed to create ConsumerMetrics: %v", err)
				return false
			}

			ctx := context.Background()

			// Clamp duration to non-negative (matching production behavior)
			if duration < 0 {
				duration = 0
			}

			switch outcome {
			case 0: // success
				cm.RecordProcessingSuccess(ctx, duration, nil, topic, partition)
			case 1: // failure
				cm.RecordProcessingFailure(ctx, duration, nil, topic, partition)
			case 2: // cancellation — no recording method called (simulates cancelled context)
				// No metrics are recorded for cancellations
			}

			// Collect metrics
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			// Extract counter values
			var processedCount, failedCount int64
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					switch m.Name {
					case "kafka_consumer_messages_processed_total":
						if sumData, ok := m.Data.(metricdata.Sum[int64]); ok {
							for _, dp := range sumData.DataPoints {
								processedCount += dp.Value
							}
						}
					case "kafka_consumer_messages_failed_total":
						if sumData, ok := m.Data.(metricdata.Sum[int64]); ok {
							for _, dp := range sumData.DataPoints {
								failedCount += dp.Value
							}
						}
					}
				}
			}

			switch outcome {
			case 0: // success: processed incremented by 1, failed NOT incremented
				return processedCount == 1 && failedCount == 0
			case 1: // failure: failed incremented by 1, processed NOT incremented
				return failedCount == 1 && processedCount == 0
			case 2: // cancellation: neither counter incremented
				return processedCount == 0 && failedCount == 0
			}
			return false
		},
		gen.IntRange(0, 2), // outcome: success, failure, or cancellation
		gen.AlphaString().Map(func(s string) string { // topic: non-empty alpha string
			if s == "" {
				return "default-topic"
			}
			return s
		}),
		gen.IntRange(0, 100),        // partition
		gen.Float64Range(0.0, 60.0), // duration in seconds
	))

	properties.TestingRun(t)
}
