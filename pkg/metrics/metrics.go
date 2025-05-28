package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all application metrics
type Metrics struct {
	// Message processing metrics
	MessagesProcessedTotal prometheus.Counter
	MessagesFailedTotal    prometheus.Counter
	MessageProcessingTime  prometheus.Histogram
	MessageProcessingGauge prometheus.Gauge

	// API call metrics
	APICallsTotal    prometheus.CounterVec
	APICallDuration  prometheus.HistogramVec
	APICallsInFlight prometheus.Gauge

	// Kafka metrics
	KafkaMessagesConsumed prometheus.Counter
	KafkaConsumerLag      prometheus.Gauge
	KafkaConnectionErrors prometheus.Counter

	// Circuit breaker metrics
	CircuitBreakerState      prometheus.GaugeVec
	CircuitBreakerOperations prometheus.CounterVec

	// Health metrics
	HealthCheckStatus   prometheus.GaugeVec
	HealthCheckDuration prometheus.HistogramVec

	// System metrics
	ActiveGoroutines prometheus.Gauge
	MemoryUsage      prometheus.Gauge
	CPUUsage         prometheus.Gauge
}

// Config represents metrics configuration
type Config struct {
	Namespace string
	Enabled   bool
}

// New creates a new metrics instance
func New(config Config) *Metrics {
	if !config.Enabled {
		return &Metrics{} // Return empty metrics if disabled
	}

	namespace := config.Namespace
	if namespace == "" {
		namespace = "confirmation"
	}

	// Create a new registry for testing to avoid conflicts
	registry := prometheus.NewRegistry()
	factory := promauto.With(registry)

	return &Metrics{
		// Message processing metrics
		MessagesProcessedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "messages_processed_total",
			Help:      "Total number of messages processed",
		}),
		MessagesFailedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "messages_failed_total",
			Help:      "Total number of messages that failed processing",
		}),
		MessageProcessingTime: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "message_processing_duration_seconds",
			Help:      "Time spent processing messages",
			Buckets:   prometheus.DefBuckets,
		}),
		MessageProcessingGauge: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "messages_processing_current",
			Help:      "Current number of messages being processed",
		}),

		// API call metrics
		APICallsTotal: *factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "api_calls_total",
			Help:      "Total number of API calls made",
		}, []string{"method", "endpoint", "status_code"}),
		APICallDuration: *factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "api_call_duration_seconds",
			Help:      "Duration of API calls",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"method", "endpoint"}),
		APICallsInFlight: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "api_calls_in_flight",
			Help:      "Current number of API calls in flight",
		}),

		// Kafka metrics
		KafkaMessagesConsumed: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_messages_consumed_total",
			Help:      "Total number of Kafka messages consumed",
		}),
		KafkaConsumerLag: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "kafka_consumer_lag",
			Help:      "Current Kafka consumer lag",
		}),
		KafkaConnectionErrors: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_connection_errors_total",
			Help:      "Total number of Kafka connection errors",
		}),

		// Circuit breaker metrics
		CircuitBreakerState: *factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		}, []string{"name"}),
		CircuitBreakerOperations: *factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "circuit_breaker_operations_total",
			Help:      "Total circuit breaker operations",
		}, []string{"name", "result"}),

		// Health metrics
		HealthCheckStatus: *factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "health_check_status",
			Help:      "Health check status (1=healthy, 0=unhealthy)",
		}, []string{"check_name"}),
		HealthCheckDuration: *factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "health_check_duration_seconds",
			Help:      "Duration of health checks",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}, []string{"check_name"}),

		// System metrics
		ActiveGoroutines: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "goroutines_active",
			Help:      "Number of active goroutines",
		}),
		MemoryUsage: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "memory_usage_bytes",
			Help:      "Current memory usage in bytes",
		}),
		CPUUsage: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cpu_usage_percent",
			Help:      "Current CPU usage percentage",
		}),
	}
}

// RecordMessageProcessed increments the processed messages counter
func (m *Metrics) RecordMessageProcessed() {
	if m.MessagesProcessedTotal != nil {
		m.MessagesProcessedTotal.Inc()
	}
}

// RecordMessageFailed increments the failed messages counter
func (m *Metrics) RecordMessageFailed() {
	if m.MessagesFailedTotal != nil {
		m.MessagesFailedTotal.Inc()
	}
}

// RecordMessageProcessingTime records the time taken to process a message
func (m *Metrics) RecordMessageProcessingTime(duration time.Duration) {
	if m.MessageProcessingTime != nil {
		m.MessageProcessingTime.Observe(duration.Seconds())
	}
}

// SetMessagesProcessing sets the current number of messages being processed
func (m *Metrics) SetMessagesProcessing(count float64) {
	if m.MessageProcessingGauge != nil {
		m.MessageProcessingGauge.Set(count)
	}
}

// RecordAPICall records an API call with method, endpoint, and status code
func (m *Metrics) RecordAPICall(method, endpoint, statusCode string, duration time.Duration) {
	if m.APICallsTotal.MetricVec != nil {
		m.APICallsTotal.WithLabelValues(method, endpoint, statusCode).Inc()
	}
	if m.APICallDuration.MetricVec != nil {
		m.APICallDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
	}
}

// IncAPICallsInFlight increments the in-flight API calls gauge
func (m *Metrics) IncAPICallsInFlight() {
	if m.APICallsInFlight != nil {
		m.APICallsInFlight.Inc()
	}
}

// DecAPICallsInFlight decrements the in-flight API calls gauge
func (m *Metrics) DecAPICallsInFlight() {
	if m.APICallsInFlight != nil {
		m.APICallsInFlight.Dec()
	}
}

// RecordKafkaMessage increments the Kafka messages consumed counter
func (m *Metrics) RecordKafkaMessage() {
	if m.KafkaMessagesConsumed != nil {
		m.KafkaMessagesConsumed.Inc()
	}
}

// SetKafkaConsumerLag sets the current Kafka consumer lag
func (m *Metrics) SetKafkaConsumerLag(lag float64) {
	if m.KafkaConsumerLag != nil {
		m.KafkaConsumerLag.Set(lag)
	}
}

// RecordKafkaConnectionError increments the Kafka connection errors counter
func (m *Metrics) RecordKafkaConnectionError() {
	if m.KafkaConnectionErrors != nil {
		m.KafkaConnectionErrors.Inc()
	}
}

// SetCircuitBreakerState sets the circuit breaker state
func (m *Metrics) SetCircuitBreakerState(name string, state float64) {
	if m.CircuitBreakerState.MetricVec != nil {
		m.CircuitBreakerState.WithLabelValues(name).Set(state)
	}
}

// RecordCircuitBreakerOperation records a circuit breaker operation
func (m *Metrics) RecordCircuitBreakerOperation(name, result string) {
	if m.CircuitBreakerOperations.MetricVec != nil {
		m.CircuitBreakerOperations.WithLabelValues(name, result).Inc()
	}
}

// SetHealthCheckStatus sets the health check status
func (m *Metrics) SetHealthCheckStatus(checkName string, healthy bool) {
	if m.HealthCheckStatus.MetricVec != nil {
		status := 0.0
		if healthy {
			status = 1.0
		}
		m.HealthCheckStatus.WithLabelValues(checkName).Set(status)
	}
}

// RecordHealthCheckDuration records the duration of a health check
func (m *Metrics) RecordHealthCheckDuration(checkName string, duration time.Duration) {
	if m.HealthCheckDuration.MetricVec != nil {
		m.HealthCheckDuration.WithLabelValues(checkName).Observe(duration.Seconds())
	}
}

// SetActiveGoroutines sets the number of active goroutines
func (m *Metrics) SetActiveGoroutines(count float64) {
	if m.ActiveGoroutines != nil {
		m.ActiveGoroutines.Set(count)
	}
}

// SetMemoryUsage sets the current memory usage
func (m *Metrics) SetMemoryUsage(bytes float64) {
	if m.MemoryUsage != nil {
		m.MemoryUsage.Set(bytes)
	}
}

// SetCPUUsage sets the current CPU usage percentage
func (m *Metrics) SetCPUUsage(percent float64) {
	if m.CPUUsage != nil {
		m.CPUUsage.Set(percent)
	}
}
