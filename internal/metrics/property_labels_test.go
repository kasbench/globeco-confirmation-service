package metrics

import (
	"context"
	"strconv"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// alphaStringMinLen generates an alphabetic string of at least minLen characters.
// Uses a fixed-size slice of AlphaChar to guarantee non-empty output without discards.
func alphaStringMinLen(minLen, maxLen int) gopter.Gen {
	return gen.SliceOfN(maxLen, gen.AlphaChar()).Map(func(chars []rune) string {
		if len(chars) < minLen {
			return string(chars) + "a" // ensure minimum length
		}
		return string(chars)
	})
}

// TestPropertyCommonLabelsAlwaysPresent validates Property 1: Common labels always present and correct.
//
// **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.6, 2.8, 2.9**
//
// For any metric observation produced by the Kafka consumer processing path,
// the observation SHALL contain the labels `service` (with value "globeco-confirmation-service"),
// `consumer_group` (with the configured value), `topic`, and `partition` with correct values.
// Additionally, NO high-cardinality labels (message_key, offset, hostname, etc.) are present.
func TestPropertyCommonLabelsAlwaysPresent(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// High-cardinality labels that must NOT be present
	forbiddenLabels := []string{
		"message_key", "offset", "hostname", "pod_uid",
		"order_id", "portfolio_id", "message_id",
	}

	properties.Property("all four common labels present with correct values on every observation from RecordPollSuccess", prop.ForAll(
		func(consumerGroup string, topic string, partition int) bool {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer provider.Shutdown(context.Background())

			meter := provider.Meter("test")
			cm, err := NewConsumerMetrics(meter, consumerGroup)
			if err != nil {
				t.Logf("failed to create ConsumerMetrics: %v", err)
				return false
			}

			ctx := context.Background()
			cm.RecordPollSuccess(ctx, 0.1, topic, partition)

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			expectedPartition := strconv.Itoa(partition)

			return verifyAllDataPointLabels(t, rm, consumerGroup, topic, expectedPartition, forbiddenLabels)
		},
		alphaStringMinLen(1, 20),
		alphaStringMinLen(1, 20),
		gen.IntRange(0, 1000),
	))

	properties.Property("all four common labels present with correct values on every observation from RecordProcessingSuccess", prop.ForAll(
		func(consumerGroup string, topic string, partition int) bool {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer provider.Shutdown(context.Background())

			meter := provider.Meter("test")
			cm, err := NewConsumerMetrics(meter, consumerGroup)
			if err != nil {
				t.Logf("failed to create ConsumerMetrics: %v", err)
				return false
			}

			ctx := context.Background()
			cm.RecordProcessingSuccess(ctx, 0.5, nil, topic, partition)

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			expectedPartition := strconv.Itoa(partition)

			return verifyAllDataPointLabels(t, rm, consumerGroup, topic, expectedPartition, forbiddenLabels)
		},
		alphaStringMinLen(1, 20),
		alphaStringMinLen(1, 20),
		gen.IntRange(0, 1000),
	))

	properties.Property("all four common labels present with correct values on every observation from RecordProcessingFailure", prop.ForAll(
		func(consumerGroup string, topic string, partition int) bool {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer provider.Shutdown(context.Background())

			meter := provider.Meter("test")
			cm, err := NewConsumerMetrics(meter, consumerGroup)
			if err != nil {
				t.Logf("failed to create ConsumerMetrics: %v", err)
				return false
			}

			ctx := context.Background()
			cm.RecordProcessingFailure(ctx, 0.3, nil, topic, partition)

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			expectedPartition := strconv.Itoa(partition)

			return verifyAllDataPointLabels(t, rm, consumerGroup, topic, expectedPartition, forbiddenLabels)
		},
		alphaStringMinLen(1, 20),
		alphaStringMinLen(1, 20),
		gen.IntRange(0, 1000),
	))

	properties.Property("all four common labels present with correct values on every observation from RecordPollError", prop.ForAll(
		func(consumerGroup string) bool {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer provider.Shutdown(context.Background())

			meter := provider.Meter("test")
			cm, err := NewConsumerMetrics(meter, consumerGroup)
			if err != nil {
				t.Logf("failed to create ConsumerMetrics: %v", err)
				return false
			}

			ctx := context.Background()
			cm.RecordPollError(ctx, 0.2)

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Logf("failed to collect metrics: %v", err)
				return false
			}

			// RecordPollError uses topic="unknown" and partition="unknown"
			return verifyAllDataPointLabels(t, rm, consumerGroup, "unknown", "unknown", forbiddenLabels)
		},
		alphaStringMinLen(1, 20),
	))

	properties.TestingRun(t)
}

// verifyAllDataPointLabels checks that every data point in the collected metrics has the
// four common labels with the expected values, and no forbidden high-cardinality labels.
func verifyAllDataPointLabels(t *testing.T, rm metricdata.ResourceMetrics, expectedConsumerGroup, expectedTopic, expectedPartition string, forbiddenLabels []string) bool {
	t.Helper()

	dataPointCount := 0

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					dataPointCount++
					if !checkAttributes(t, m.Name, dp.Attributes, expectedConsumerGroup, expectedTopic, expectedPartition, forbiddenLabels) {
						return false
					}
				}
			case metricdata.Sum[float64]:
				for _, dp := range data.DataPoints {
					dataPointCount++
					if !checkAttributes(t, m.Name, dp.Attributes, expectedConsumerGroup, expectedTopic, expectedPartition, forbiddenLabels) {
						return false
					}
				}
			case metricdata.Histogram[float64]:
				for _, dp := range data.DataPoints {
					dataPointCount++
					if !checkAttributes(t, m.Name, dp.Attributes, expectedConsumerGroup, expectedTopic, expectedPartition, forbiddenLabels) {
						return false
					}
				}
			}
		}
	}

	if dataPointCount == 0 {
		t.Logf("no data points found in collected metrics")
		return false
	}

	return true
}

// checkAttributes verifies that the given attribute set contains the four common labels
// with expected values, and does not contain any forbidden labels.
func checkAttributes(t *testing.T, metricName string, attrs attribute.Set, expectedConsumerGroup, expectedTopic, expectedPartition string, forbiddenLabels []string) bool {
	t.Helper()

	// Verify "service" label
	serviceVal, ok := attrs.Value(attribute.Key("service"))
	if !ok {
		t.Logf("metric %s: missing 'service' label", metricName)
		return false
	}
	if serviceVal.AsString() != "globeco-confirmation-service" {
		t.Logf("metric %s: 'service' label has value %q, want %q", metricName, serviceVal.AsString(), "globeco-confirmation-service")
		return false
	}

	// Verify "consumer_group" label
	cgVal, ok := attrs.Value(attribute.Key("consumer_group"))
	if !ok {
		t.Logf("metric %s: missing 'consumer_group' label", metricName)
		return false
	}
	if cgVal.AsString() != expectedConsumerGroup {
		t.Logf("metric %s: 'consumer_group' label has value %q, want %q", metricName, cgVal.AsString(), expectedConsumerGroup)
		return false
	}

	// Verify "topic" label
	topicVal, ok := attrs.Value(attribute.Key("topic"))
	if !ok {
		t.Logf("metric %s: missing 'topic' label", metricName)
		return false
	}
	if topicVal.AsString() != expectedTopic {
		t.Logf("metric %s: 'topic' label has value %q, want %q", metricName, topicVal.AsString(), expectedTopic)
		return false
	}

	// Verify "partition" label
	partVal, ok := attrs.Value(attribute.Key("partition"))
	if !ok {
		t.Logf("metric %s: missing 'partition' label", metricName)
		return false
	}
	if partVal.AsString() != expectedPartition {
		t.Logf("metric %s: 'partition' label has value %q, want %q", metricName, partVal.AsString(), expectedPartition)
		return false
	}

	// Verify NO high-cardinality forbidden labels
	for _, forbidden := range forbiddenLabels {
		if _, exists := attrs.Value(attribute.Key(forbidden)); exists {
			t.Logf("metric %s: found forbidden high-cardinality label %q", metricName, forbidden)
			return false
		}
	}

	return true
}
