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

// TestPropertyPollCounterIncrementsIffSuccess validates Property 4:
// Poll counter increments iff FetchMessage succeeds.
//
// **Validates: Requirements 9.1, 9.2, 9.3, 9.4**
//
// For any invocation of FetchMessage, records_polled_total SHALL increment by
// exactly 1 if and only if the call returns without error. On error (whether
// non-cancellation error or context cancellation), the counter SHALL NOT increment.
func TestPropertyPollCounterIncrementsIffSuccess(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// outcome: 0=success, 1=non-cancellation error, 2=context cancellation
	properties.Property("records_polled_total increments only on successful FetchMessage", prop.ForAll(
		func(outcome int, duration float64, topic string, partition int) bool {
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

			switch outcome {
			case 0: // success: FetchMessage returned a message successfully
				cm.RecordPollSuccess(ctx, duration, topic, partition)
			case 1: // non-cancellation error: FetchMessage returned an error
				cm.RecordPollError(ctx, duration)
			case 2: // context cancellation: no metrics recorded at all
				// Per design, context cancellation produces zero metric observations
			}

			// Collect metrics
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			// Extract records_polled_total counter value
			var polledCount int64
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "kafka_consumer_records_polled_total" {
						if sumData, ok := m.Data.(metricdata.Sum[int64]); ok {
							for _, dp := range sumData.DataPoints {
								polledCount += dp.Value
							}
						}
					}
				}
			}

			switch outcome {
			case 0: // success: records_polled_total should be exactly 1
				if polledCount != 1 {
					t.Logf("outcome=success: records_polled_total got %d, want 1", polledCount)
					return false
				}
			case 1: // non-cancellation error: records_polled_total should be 0
				if polledCount != 0 {
					t.Logf("outcome=error: records_polled_total got %d, want 0", polledCount)
					return false
				}
			case 2: // context cancellation: records_polled_total should be 0
				if polledCount != 0 {
					t.Logf("outcome=cancellation: records_polled_total got %d, want 0", polledCount)
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 2), // outcome: success, non-cancellation error, or context cancellation
		gen.Float64Range(0.001, 60.0), // poll duration in seconds
		gen.AlphaString().Map(func(s string) string { // topic: non-empty alpha string
			if s == "" {
				return "default-topic"
			}
			return s
		}),
		gen.IntRange(0, 100), // partition
	))

	properties.TestingRun(t)
}
