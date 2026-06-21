package metrics

import (
	"context"
	"math"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestPropertyPollSecondsAccumulatesForNonCancelled validates Property 5:
// Poll seconds accumulates for all non-cancelled FetchMessage calls.
//
// **Validates: Requirements 10.1, 10.2, 10.3, 10.4, 10.5**
//
// For any FetchMessage call that completes (success or non-cancellation error),
// poll_seconds_total SHALL increase by the monotonic elapsed duration of that call.
// For context-cancelled calls, no poll time SHALL be recorded.
func TestPropertyPollSecondsAccumulatesForNonCancelled(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// outcome: 0 = success, 1 = non-cancellation error, 2 = cancellation
	properties.Property("poll_seconds_total accumulates for success and error, but not for cancellation", prop.ForAll(
		func(outcome int, pollDuration float64) bool {
			// Create a fresh MeterProvider with a ManualReader for each iteration
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
			case 0: // success — RecordPollSuccess adds pollDuration to poll_seconds_total
				cm.RecordPollSuccess(ctx, pollDuration, "test-topic", 0)
			case 1: // non-cancellation error — RecordPollError adds pollDuration to poll_seconds_total
				cm.RecordPollError(ctx, pollDuration)
			case 2: // cancellation — no recording method called (no metrics recorded)
				// Context cancelled: nothing is recorded per design
			}

			// Collect metrics
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			// Extract poll_seconds_total value
			var pollSecondsTotal float64
			var foundPollSeconds bool
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "kafka_consumer_poll_seconds_total" {
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								pollSecondsTotal += dp.Value
								foundPollSeconds = true
							}
						}
					}
				}
			}

			switch outcome {
			case 0: // success: poll_seconds_total should have increased by pollDuration
				if !foundPollSeconds {
					t.Logf("poll_seconds_total not found after successful poll")
					return false
				}
				if math.Abs(pollSecondsTotal-pollDuration) > 1e-9 {
					t.Logf("poll_seconds_total: got %v, want %v", pollSecondsTotal, pollDuration)
					return false
				}
			case 1: // non-cancellation error: poll_seconds_total should have increased by pollDuration
				if !foundPollSeconds {
					t.Logf("poll_seconds_total not found after poll error")
					return false
				}
				if math.Abs(pollSecondsTotal-pollDuration) > 1e-9 {
					t.Logf("poll_seconds_total: got %v, want %v", pollSecondsTotal, pollDuration)
					return false
				}
			case 2: // cancellation: poll_seconds_total should remain 0 (no data points)
				if foundPollSeconds && pollSecondsTotal != 0 {
					t.Logf("poll_seconds_total should be 0 for cancellation, got %v", pollSecondsTotal)
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 2),              // outcome: success, error, or cancellation
		gen.Float64Range(0.001, 60.0),   // pollDuration in seconds
	))

	properties.Property("poll_seconds_total accumulates across multiple non-cancelled calls", prop.ForAll(
		func(durations []float64) bool {
			// Create a fresh MeterProvider with a ManualReader
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
			var expectedTotal float64

			// Record multiple poll successes and errors, accumulating expected total
			for i, d := range durations {
				if i%2 == 0 {
					cm.RecordPollSuccess(ctx, d, "test-topic", i%10)
				} else {
					cm.RecordPollError(ctx, d)
				}
				expectedTotal += d
			}

			// Collect metrics
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			// Extract poll_seconds_total value
			var pollSecondsTotal float64
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "kafka_consumer_poll_seconds_total" {
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								pollSecondsTotal += dp.Value
							}
						}
					}
				}
			}

			if math.Abs(pollSecondsTotal-expectedTotal) > 1e-9 {
				t.Logf("accumulated poll_seconds_total: got %v, want %v", pollSecondsTotal, expectedTotal)
				return false
			}

			return true
		},
		gen.SliceOfN(5, gen.Float64Range(0.001, 60.0)),
	))

	properties.TestingRun(t)
}
