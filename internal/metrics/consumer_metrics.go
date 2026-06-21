package metrics

import (
	"context"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ConsumerMetrics holds all Kafka consumer metric instruments.
// All fields are immutable after construction; safe for concurrent use.
type ConsumerMetrics struct {
	messagesProcessed  metric.Int64Counter
	messagesFailed     metric.Int64Counter
	processingSeconds  metric.Float64Counter
	idleSeconds        metric.Float64Counter
	recordsPolled      metric.Int64Counter
	pollSeconds        metric.Float64Counter
	processingDuration metric.Float64Histogram
	messageLatency     metric.Float64Histogram

	// Pre-computed common attributes (service + consumer_group).
	// Topic and partition are added per-observation.
	commonAttrs []attribute.KeyValue
}

// NewConsumerMetrics creates and registers all metric instruments.
// Returns an error if any instrument cannot be created.
func NewConsumerMetrics(meter metric.Meter, consumerGroup string) (*ConsumerMetrics, error) {
	if consumerGroup == "" {
		consumerGroup = "unknown"
	}

	commonAttrs := []attribute.KeyValue{
		attribute.String("service", "globeco-confirmation-service"),
		attribute.String("consumer_group", consumerGroup),
	}

	messagesProcessed, err := meter.Int64Counter(
		"kafka_consumer_messages_processed_total",
		metric.WithUnit("1"),
		metric.WithDescription("Total number of successfully processed Kafka messages"),
	)
	if err != nil {
		return nil, err
	}

	messagesFailed, err := meter.Int64Counter(
		"kafka_consumer_messages_failed_total",
		metric.WithUnit("1"),
		metric.WithDescription("Total number of terminally failed Kafka messages"),
	)
	if err != nil {
		return nil, err
	}

	processingSeconds, err := meter.Float64Counter(
		"kafka_consumer_processing_seconds_total",
		metric.WithUnit("s"),
		metric.WithDescription("Total active processing time in seconds"),
	)
	if err != nil {
		return nil, err
	}

	idleSeconds, err := meter.Float64Counter(
		"kafka_consumer_idle_seconds_total",
		metric.WithUnit("s"),
		metric.WithDescription("Total idle time spent waiting for messages in seconds"),
	)
	if err != nil {
		return nil, err
	}

	recordsPolled, err := meter.Int64Counter(
		"kafka_consumer_records_polled_total",
		metric.WithUnit("1"),
		metric.WithDescription("Total number of records returned by Kafka poll operations"),
	)
	if err != nil {
		return nil, err
	}

	pollSeconds, err := meter.Float64Counter(
		"kafka_consumer_poll_seconds_total",
		metric.WithUnit("s"),
		metric.WithDescription("Total time spent in Kafka poll operations in seconds"),
	)
	if err != nil {
		return nil, err
	}

	processingDuration, err := meter.Float64Histogram(
		"kafka_consumer_processing_duration_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Distribution of per-message processing times in seconds"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60,
		),
	)
	if err != nil {
		return nil, err
	}

	messageLatency, err := meter.Float64Histogram(
		"kafka_consumer_message_latency_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Distribution of end-to-end message delivery latency in seconds"),
		metric.WithExplicitBucketBoundaries(
			0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60, 120, 300, 600,
		),
	)
	if err != nil {
		return nil, err
	}

	return &ConsumerMetrics{
		messagesProcessed:  messagesProcessed,
		messagesFailed:     messagesFailed,
		processingSeconds:  processingSeconds,
		idleSeconds:        idleSeconds,
		recordsPolled:      recordsPolled,
		pollSeconds:        pollSeconds,
		processingDuration: processingDuration,
		messageLatency:     messageLatency,
		commonAttrs:        commonAttrs,
	}, nil
}

// partitionLabel returns the string representation of a partition number.
// Returns "unknown" if the partition is negative (invalid).
func partitionLabel(partition int) string {
	if partition < 0 {
		return "unknown"
	}
	return strconv.Itoa(partition)
}

// RecordPollSuccess records metrics after a successful FetchMessage call.
// Increments records_polled_total, adds pollDuration to poll_seconds_total and idle_seconds_total.
// Called from the fetchLoop goroutine only.
func (m *ConsumerMetrics) RecordPollSuccess(ctx context.Context, pollDuration float64, topic string, partition int) {
	defer func() { recover() }()

	allAttrs := make([]attribute.KeyValue, len(m.commonAttrs), len(m.commonAttrs)+2)
	copy(allAttrs, m.commonAttrs)
	allAttrs = append(allAttrs,
		attribute.String("topic", topic),
		attribute.String("partition", partitionLabel(partition)),
	)
	attrs := metric.WithAttributes(allAttrs...)

	m.recordsPolled.Add(ctx, 1, attrs)
	m.pollSeconds.Add(ctx, pollDuration, attrs)
	m.idleSeconds.Add(ctx, pollDuration, attrs)
}

// RecordPollError records metrics after a failed FetchMessage (non-cancellation).
// Adds pollDuration to poll_seconds_total and idle_seconds_total with topic="unknown" and partition="unknown".
// Called from the fetchLoop goroutine only.
func (m *ConsumerMetrics) RecordPollError(ctx context.Context, pollDuration float64) {
	defer func() { recover() }()

	allAttrs := make([]attribute.KeyValue, len(m.commonAttrs), len(m.commonAttrs)+2)
	copy(allAttrs, m.commonAttrs)
	allAttrs = append(allAttrs,
		attribute.String("topic", "unknown"),
		attribute.String("partition", "unknown"),
	)
	attrs := metric.WithAttributes(allAttrs...)

	m.pollSeconds.Add(ctx, pollDuration, attrs)
	m.idleSeconds.Add(ctx, pollDuration, attrs)
}

// RecordProcessingSuccess records metrics after successful message processing.
// Increments messages_processed_total, adds processingDuration to processing_seconds_total,
// records processing_duration_seconds histogram with result=success,
// and records message_latency_seconds histogram with result=success if latency != nil.
// Called from workerLoop goroutines.
func (m *ConsumerMetrics) RecordProcessingSuccess(ctx context.Context, processingDuration float64, latency *float64, topic string, partition int) {
	defer func() { recover() }()

	partLabel := partitionLabel(partition)

	counterAttrs := make([]attribute.KeyValue, len(m.commonAttrs), len(m.commonAttrs)+2)
	copy(counterAttrs, m.commonAttrs)
	counterAttrs = append(counterAttrs,
		attribute.String("topic", topic),
		attribute.String("partition", partLabel),
	)

	histAttrs := make([]attribute.KeyValue, len(m.commonAttrs), len(m.commonAttrs)+3)
	copy(histAttrs, m.commonAttrs)
	histAttrs = append(histAttrs,
		attribute.String("topic", topic),
		attribute.String("partition", partLabel),
		attribute.String("result", "success"),
	)

	m.messagesProcessed.Add(ctx, 1, metric.WithAttributes(counterAttrs...))
	m.processingSeconds.Add(ctx, processingDuration, metric.WithAttributes(counterAttrs...))
	m.processingDuration.Record(ctx, processingDuration, metric.WithAttributes(histAttrs...))

	if latency != nil {
		m.messageLatency.Record(ctx, *latency, metric.WithAttributes(histAttrs...))
	}
}

// RecordProcessingFailure records metrics after failed message processing.
// Increments messages_failed_total, adds processingDuration to processing_seconds_total,
// records processing_duration_seconds histogram with result=failure,
// and records message_latency_seconds histogram with result=failure if latency != nil.
// Called from workerLoop goroutines.
func (m *ConsumerMetrics) RecordProcessingFailure(ctx context.Context, processingDuration float64, latency *float64, topic string, partition int) {
	defer func() { recover() }()

	partLabel := partitionLabel(partition)

	counterAttrs := make([]attribute.KeyValue, len(m.commonAttrs), len(m.commonAttrs)+2)
	copy(counterAttrs, m.commonAttrs)
	counterAttrs = append(counterAttrs,
		attribute.String("topic", topic),
		attribute.String("partition", partLabel),
	)

	histAttrs := make([]attribute.KeyValue, len(m.commonAttrs), len(m.commonAttrs)+3)
	copy(histAttrs, m.commonAttrs)
	histAttrs = append(histAttrs,
		attribute.String("topic", topic),
		attribute.String("partition", partLabel),
		attribute.String("result", "failure"),
	)

	m.messagesFailed.Add(ctx, 1, metric.WithAttributes(counterAttrs...))
	m.processingSeconds.Add(ctx, processingDuration, metric.WithAttributes(counterAttrs...))
	m.processingDuration.Record(ctx, processingDuration, metric.WithAttributes(histAttrs...))

	if latency != nil {
		m.messageLatency.Record(ctx, *latency, metric.WithAttributes(histAttrs...))
	}
}
