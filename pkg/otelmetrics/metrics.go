package otelmetrics

import (
	"context"
	"runtime"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics represents OpenTelemetry-based metrics for the confirmation service
type Metrics struct {
	// Message processing metrics
	messagesProcessedTotal   metric.Int64Counter
	messagesFailedTotal      metric.Int64Counter
	messageProcessingTime    metric.Float64Histogram
	messagesProcessingCurrent metric.Int64UpDownCounter

	// API call metrics
	apiCallsTotal     metric.Int64Counter
	apiCallDuration   metric.Float64Histogram
	apiCallsInFlight  metric.Int64UpDownCounter

	// Kafka metrics
	kafkaMessagesConsumed   metric.Int64Counter
	kafkaConsumerLag        metric.Float64Gauge
	kafkaConnectionErrors   metric.Int64Counter

	// Circuit breaker metrics
	circuitBreakerState      metric.Int64Gauge
	circuitBreakerOperations metric.Int64Counter

	// Health metrics
	healthCheckStatus   metric.Int64Gauge
	healthCheckDuration metric.Float64Histogram

	// System metrics
	activeGoroutines metric.Int64Gauge
	memoryUsage      metric.Int64Gauge
	cpuUsage         metric.Float64Gauge

	enabled bool
	meter   metric.Meter
}

// Config represents metrics configuration
type Config struct {
	ServiceName string
	Enabled     bool
}

// New creates a new OpenTelemetry-based metrics instance
func New(config Config) *Metrics {
	if !config.Enabled {
		return &Metrics{enabled: false}
	}

	meter := otel.Meter(config.ServiceName)

	// Create all metrics
	messagesProcessedTotal, _ := meter.Int64Counter(
		"messages_processed_total",
		metric.WithDescription("Total number of messages processed"),
	)

	messagesFailedTotal, _ := meter.Int64Counter(
		"messages_failed_total",
		metric.WithDescription("Total number of messages that failed processing"),
	)

	messageProcessingTime, _ := meter.Float64Histogram(
		"message_processing_duration_seconds",
		metric.WithDescription("Time spent processing messages"),
		metric.WithUnit("s"),
	)

	messagesProcessingCurrent, _ := meter.Int64UpDownCounter(
		"messages_processing_current",
		metric.WithDescription("Number of messages currently being processed"),
	)

	apiCallsTotal, _ := meter.Int64Counter(
		"api_calls_total",
		metric.WithDescription("Total number of API calls made"),
	)

	apiCallDuration, _ := meter.Float64Histogram(
		"api_call_duration_seconds",
		metric.WithDescription("Duration of API calls"),
		metric.WithUnit("s"),
	)

	apiCallsInFlight, _ := meter.Int64UpDownCounter(
		"api_calls_in_flight",
		metric.WithDescription("Number of API calls currently in flight"),
	)

	kafkaMessagesConsumed, _ := meter.Int64Counter(
		"kafka_messages_consumed_total",
		metric.WithDescription("Total number of Kafka messages consumed"),
	)

	kafkaConsumerLag, _ := meter.Float64Gauge(
		"kafka_consumer_lag",
		metric.WithDescription("Current Kafka consumer lag"),
	)

	kafkaConnectionErrors, _ := meter.Int64Counter(
		"kafka_connection_errors_total",
		metric.WithDescription("Total number of Kafka connection errors"),
	)

	circuitBreakerState, _ := meter.Int64Gauge(
		"circuit_breaker_state",
		metric.WithDescription("Circuit breaker state (0=closed, 1=open, 2=half-open)"),
	)

	circuitBreakerOperations, _ := meter.Int64Counter(
		"circuit_breaker_operations_total",
		metric.WithDescription("Total number of circuit breaker operations"),
	)

	healthCheckStatus, _ := meter.Int64Gauge(
		"health_check_status",
		metric.WithDescription("Health check status (1=healthy, 0=unhealthy)"),
	)

	healthCheckDuration, _ := meter.Float64Histogram(
		"health_check_duration_seconds",
		metric.WithDescription("Duration of health checks"),
		metric.WithUnit("s"),
	)

	activeGoroutines, _ := meter.Int64Gauge(
		"goroutines_active",
		metric.WithDescription("Number of active goroutines"),
	)

	memoryUsage, _ := meter.Int64Gauge(
		"memory_usage_bytes",
		metric.WithDescription("Current memory usage in bytes"),
	)

	cpuUsage, _ := meter.Float64Gauge(
		"cpu_usage_percent",
		metric.WithDescription("Current CPU usage percentage"),
	)

	return &Metrics{
		messagesProcessedTotal:    messagesProcessedTotal,
		messagesFailedTotal:       messagesFailedTotal,
		messageProcessingTime:     messageProcessingTime,
		messagesProcessingCurrent: messagesProcessingCurrent,
		apiCallsTotal:             apiCallsTotal,
		apiCallDuration:           apiCallDuration,
		apiCallsInFlight:          apiCallsInFlight,
		kafkaMessagesConsumed:     kafkaMessagesConsumed,
		kafkaConsumerLag:          kafkaConsumerLag,
		kafkaConnectionErrors:     kafkaConnectionErrors,
		circuitBreakerState:       circuitBreakerState,
		circuitBreakerOperations:  circuitBreakerOperations,
		healthCheckStatus:         healthCheckStatus,
		healthCheckDuration:       healthCheckDuration,
		activeGoroutines:          activeGoroutines,
		memoryUsage:               memoryUsage,
		cpuUsage:                  cpuUsage,
		enabled:                   true,
		meter:                     meter,
	}
}

// RecordMessageProcessed increments the processed messages counter
func (m *Metrics) RecordMessageProcessed(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.messagesProcessedTotal.Add(ctx, 1)
}

// RecordMessageFailed increments the failed messages counter
func (m *Metrics) RecordMessageFailed(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.messagesFailedTotal.Add(ctx, 1)
}

// RecordMessageProcessingTime records the time spent processing a message
func (m *Metrics) RecordMessageProcessingTime(ctx context.Context, duration time.Duration) {
	if !m.enabled {
		return
	}
	m.messageProcessingTime.Record(ctx, duration.Seconds())
}

// IncMessagesProcessingCurrent increments the current processing counter
func (m *Metrics) IncMessagesProcessingCurrent(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.messagesProcessingCurrent.Add(ctx, 1)
}

// DecMessagesProcessingCurrent decrements the current processing counter
func (m *Metrics) DecMessagesProcessingCurrent(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.messagesProcessingCurrent.Add(ctx, -1)
}

// RecordAPICall records an API call with method, endpoint, and status code
func (m *Metrics) RecordAPICall(ctx context.Context, method, endpoint, statusCode string, duration time.Duration) {
	if !m.enabled {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("endpoint", endpoint),
		attribute.String("status_code", statusCode),
	}
	m.apiCallsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	
	durationAttrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("endpoint", endpoint),
	}
	m.apiCallDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(durationAttrs...))
}

// IncAPICallsInFlight increments the in-flight API calls counter
func (m *Metrics) IncAPICallsInFlight(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.apiCallsInFlight.Add(ctx, 1)
}

// DecAPICallsInFlight decrements the in-flight API calls counter
func (m *Metrics) DecAPICallsInFlight(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.apiCallsInFlight.Add(ctx, -1)
}

// RecordKafkaMessage increments the Kafka messages consumed counter
func (m *Metrics) RecordKafkaMessage(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.kafkaMessagesConsumed.Add(ctx, 1)
}

// SetKafkaConsumerLag sets the current Kafka consumer lag
func (m *Metrics) SetKafkaConsumerLag(ctx context.Context, lag float64) {
	if !m.enabled {
		return
	}
	m.kafkaConsumerLag.Record(ctx, lag)
}

// RecordKafkaConnectionError increments the Kafka connection errors counter
func (m *Metrics) RecordKafkaConnectionError(ctx context.Context) {
	if !m.enabled {
		return
	}
	m.kafkaConnectionErrors.Add(ctx, 1)
}

// SetCircuitBreakerState sets the circuit breaker state
func (m *Metrics) SetCircuitBreakerState(ctx context.Context, name string, state int64) {
	if !m.enabled {
		return
	}
	attrs := []attribute.KeyValue{attribute.String("name", name)}
	m.circuitBreakerState.Record(ctx, state, metric.WithAttributes(attrs...))
}

// RecordCircuitBreakerOperation records a circuit breaker operation
func (m *Metrics) RecordCircuitBreakerOperation(ctx context.Context, name, result string) {
	if !m.enabled {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("name", name),
		attribute.String("result", result),
	}
	m.circuitBreakerOperations.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// SetHealthCheckStatus sets the health check status
func (m *Metrics) SetHealthCheckStatus(ctx context.Context, checkName string, status int64) {
	if !m.enabled {
		return
	}
	attrs := []attribute.KeyValue{attribute.String("check_name", checkName)}
	m.healthCheckStatus.Record(ctx, status, metric.WithAttributes(attrs...))
}

// RecordHealthCheckDuration records the duration of a health check
func (m *Metrics) RecordHealthCheckDuration(ctx context.Context, checkName string, duration time.Duration) {
	if !m.enabled {
		return
	}
	attrs := []attribute.KeyValue{attribute.String("check_name", checkName)}
	m.healthCheckDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// UpdateSystemMetrics updates system-level metrics
func (m *Metrics) UpdateSystemMetrics(ctx context.Context) {
	if !m.enabled {
		return
	}

	// Update goroutines
	m.activeGoroutines.Record(ctx, int64(runtime.NumGoroutine()))

	// Update memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.memoryUsage.Record(ctx, int64(memStats.Alloc))
}

// SetMessagesProcessingCurrent sets the current number of messages being processed
func (m *Metrics) SetMessagesProcessingCurrent(ctx context.Context, count int64) {
	if !m.enabled {
		return
	}
	// Reset the counter by adding the difference
	m.messagesProcessingCurrent.Add(ctx, count)
}

// SetActiveGoroutines sets the number of active goroutines
func (m *Metrics) SetActiveGoroutines(ctx context.Context, count int64) {
	if !m.enabled {
		return
	}
	m.activeGoroutines.Record(ctx, count)
}

// SetMemoryUsage sets the current memory usage in bytes
func (m *Metrics) SetMemoryUsage(ctx context.Context, bytes int64) {
	if !m.enabled {
		return
	}
	m.memoryUsage.Record(ctx, bytes)
}

// SetCPUUsage sets the current CPU usage percentage
func (m *Metrics) SetCPUUsage(ctx context.Context, percent float64) {
	if !m.enabled {
		return
	}
	m.cpuUsage.Record(ctx, percent)
}

// IsEnabled returns whether metrics are enabled
func (m *Metrics) IsEnabled() bool {
	return m.enabled
}