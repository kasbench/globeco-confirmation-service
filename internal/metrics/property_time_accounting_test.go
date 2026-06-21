package metrics

import (
	"context"
	"math"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestPropertyTimeAccountingConservation verifies Property 3: Time accounting conservation.
// For any complete fetch-then-process cycle, idle_seconds_total receives the poll duration
// and processing_seconds_total receives the processing duration with no overlap or double-counting.
//
// **Validates: Requirements 5.1, 5.2, 5.4, 5.5, 6.1, 6.2, 6.3, 14.3**
func TestPropertyTimeAccountingConservation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("idle_seconds receives poll duration and processing_seconds receives processing duration with no overlap", prop.ForAll(
		func(pollDuration float64, processingDuration float64) bool {
			// Setup: fresh ManualReader and MeterProvider per iteration
			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			meter := provider.Meter("test")

			cm, err := NewConsumerMetrics(meter, "test-group")
			if err != nil {
				t.Logf("failed to create ConsumerMetrics: %v", err)
				return false
			}

			ctx := context.Background()

			// Simulate a poll success (adds pollDuration to idle_seconds_total)
			cm.RecordPollSuccess(ctx, pollDuration, "test-topic", 0)

			// Simulate processing success (adds processingDuration to processing_seconds_total)
			cm.RecordProcessingSuccess(ctx, processingDuration, nil, "test-topic", 0)

			// Collect metrics
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			var idleTotal float64
			var processingTotal float64
			var foundIdle, foundProcessing bool

			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					switch m.Name {
					case "kafka_consumer_idle_seconds_total":
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								idleTotal += dp.Value
								foundIdle = true
							}
						}
					case "kafka_consumer_processing_seconds_total":
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								processingTotal += dp.Value
								foundProcessing = true
							}
						}
					}
				}
			}

			if !foundIdle {
				t.Logf("idle_seconds_total not found in metrics")
				return false
			}
			if !foundProcessing {
				t.Logf("processing_seconds_total not found in metrics")
				return false
			}

			// Verify idle_seconds_total received exactly the poll duration
			if !floatEquals(idleTotal, pollDuration) {
				t.Logf("idle_seconds_total: got %v, want %v", idleTotal, pollDuration)
				return false
			}

			// Verify processing_seconds_total received exactly the processing duration
			if !floatEquals(processingTotal, processingDuration) {
				t.Logf("processing_seconds_total: got %v, want %v", processingTotal, processingDuration)
				return false
			}

			// Verify NO overlap: idle did not receive processing duration
			if pollDuration != processingDuration && floatEquals(idleTotal, processingDuration) {
				t.Logf("idle_seconds_total incorrectly received processing duration")
				return false
			}

			// Verify NO overlap: processing did not receive poll duration
			if pollDuration != processingDuration && floatEquals(processingTotal, pollDuration) {
				t.Logf("processing_seconds_total incorrectly received poll duration")
				return false
			}

			return true
		},
		gen.Float64Range(0.001, 60.0),
		gen.Float64Range(0.001, 60.0),
	))

	properties.Property("processing failure also routes duration to processing_seconds not idle_seconds", prop.ForAll(
		func(pollDuration float64, processingDuration float64) bool {
			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			meter := provider.Meter("test")

			cm, err := NewConsumerMetrics(meter, "test-group")
			if err != nil {
				return false
			}

			ctx := context.Background()

			// Poll success adds to idle
			cm.RecordPollSuccess(ctx, pollDuration, "test-topic", 1)

			// Processing failure adds to processing_seconds, NOT idle
			cm.RecordProcessingFailure(ctx, processingDuration, nil, "test-topic", 1)

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				return false
			}

			var idleTotal float64
			var processingTotal float64

			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					switch m.Name {
					case "kafka_consumer_idle_seconds_total":
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								idleTotal += dp.Value
							}
						}
					case "kafka_consumer_processing_seconds_total":
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								processingTotal += dp.Value
							}
						}
					}
				}
			}

			// idle_seconds should only contain pollDuration, not processingDuration
			if !floatEquals(idleTotal, pollDuration) {
				t.Logf("idle got %v want %v (poll only)", idleTotal, pollDuration)
				return false
			}

			// processing_seconds should only contain processingDuration, not pollDuration
			if !floatEquals(processingTotal, processingDuration) {
				t.Logf("processing got %v want %v", processingTotal, processingDuration)
				return false
			}

			return true
		},
		gen.Float64Range(0.001, 60.0),
		gen.Float64Range(0.001, 60.0),
	))

	properties.Property("poll error routes duration to idle_seconds not processing_seconds", prop.ForAll(
		func(pollDuration float64) bool {
			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			meter := provider.Meter("test")

			cm, err := NewConsumerMetrics(meter, "test-group")
			if err != nil {
				return false
			}

			ctx := context.Background()

			// Poll error adds to idle_seconds (poll time is idle time)
			cm.RecordPollError(ctx, pollDuration)

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				return false
			}

			var idleTotal float64
			var processingTotal float64
			var foundIdle bool

			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					switch m.Name {
					case "kafka_consumer_idle_seconds_total":
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								idleTotal += dp.Value
								foundIdle = true
							}
						}
					case "kafka_consumer_processing_seconds_total":
						if sumData, ok := m.Data.(metricdata.Sum[float64]); ok {
							for _, dp := range sumData.DataPoints {
								processingTotal += dp.Value
							}
						}
					}
				}
			}

			// Poll error duration goes to idle
			if !foundIdle || !floatEquals(idleTotal, pollDuration) {
				t.Logf("idle got %v want %v", idleTotal, pollDuration)
				return false
			}

			// Processing should remain zero
			if processingTotal != 0 {
				t.Logf("processing_seconds should be 0 after poll error, got %v", processingTotal)
				return false
			}

			return true
		},
		gen.Float64Range(0.001, 60.0),
	))

	properties.TestingRun(t)
}

// floatEquals compares two float64 values with a small tolerance for floating point imprecision.
func floatEquals(a, b float64) bool {
	const epsilon = 1e-9
	return math.Abs(a-b) < epsilon
}
