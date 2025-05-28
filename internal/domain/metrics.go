package domain

import (
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	// MetricTypeCounter represents a counter metric
	MetricTypeCounter MetricType = "counter"
	// MetricTypeGauge represents a gauge metric
	MetricTypeGauge MetricType = "gauge"
	// MetricTypeHistogram represents a histogram metric
	MetricTypeHistogram MetricType = "histogram"
)

// MetricLabels represents labels for metrics
type MetricLabels map[string]string

// ProcessingMetrics represents metrics for message processing
type ProcessingMetrics struct {
	MessagesProcessedTotal int64         `json:"messages_processed_total"`
	MessagesFailedTotal    int64         `json:"messages_failed_total"`
	ProcessingDuration     time.Duration `json:"processing_duration"`
	CorrelationID          string        `json:"correlation_id"`
	FillID                 int64         `json:"fill_id"`
	ExecutionServiceID     int64         `json:"execution_service_id"`
	ErrorType              string        `json:"error_type,omitempty"`
	Timestamp              time.Time     `json:"timestamp"`
}

// APIMetrics represents metrics for API calls
type APIMetrics struct {
	RequestsTotal      int64         `json:"requests_total"`
	RequestDuration    time.Duration `json:"request_duration"`
	ResponseStatusCode int           `json:"response_status_code"`
	Method             string        `json:"method"`
	Endpoint           string        `json:"endpoint"`
	Service            string        `json:"service"`
	CorrelationID      string        `json:"correlation_id"`
	ErrorType          string        `json:"error_type,omitempty"`
	Timestamp          time.Time     `json:"timestamp"`
}

// KafkaMetrics represents metrics for Kafka operations
type KafkaMetrics struct {
	MessagesConsumed int64     `json:"messages_consumed"`
	ConsumerLag      int64     `json:"consumer_lag"`
	Topic            string    `json:"topic"`
	Partition        int       `json:"partition"`
	Offset           int64     `json:"offset"`
	CorrelationID    string    `json:"correlation_id"`
	Timestamp        time.Time `json:"timestamp"`
}

// HealthMetrics represents health check metrics
type HealthMetrics struct {
	Status           string        `json:"status"`
	CheckDuration    time.Duration `json:"check_duration"`
	CheckType        string        `json:"check_type"` // "liveness" or "readiness"
	DependencyStatus string        `json:"dependency_status,omitempty"`
	DependencyName   string        `json:"dependency_name,omitempty"`
	Timestamp        time.Time     `json:"timestamp"`
}

// CircuitBreakerMetrics represents circuit breaker metrics
type CircuitBreakerMetrics struct {
	State        string    `json:"state"` // "closed", "open", "half-open"
	FailureCount int64     `json:"failure_count"`
	SuccessCount int64     `json:"success_count"`
	Service      string    `json:"service"`
	Timestamp    time.Time `json:"timestamp"`
}

// NewProcessingMetrics creates a new ProcessingMetrics instance
func NewProcessingMetrics(correlationID string, fillID, executionServiceID int64) *ProcessingMetrics {
	return &ProcessingMetrics{
		CorrelationID:      correlationID,
		FillID:             fillID,
		ExecutionServiceID: executionServiceID,
		Timestamp:          time.Now(),
	}
}

// WithSuccess marks the processing as successful
func (m *ProcessingMetrics) WithSuccess(duration time.Duration) *ProcessingMetrics {
	m.MessagesProcessedTotal = 1
	m.ProcessingDuration = duration
	return m
}

// WithFailure marks the processing as failed
func (m *ProcessingMetrics) WithFailure(duration time.Duration, errorType string) *ProcessingMetrics {
	m.MessagesFailedTotal = 1
	m.ProcessingDuration = duration
	m.ErrorType = errorType
	return m
}

// NewAPIMetrics creates a new APIMetrics instance
func NewAPIMetrics(method, endpoint, service, correlationID string) *APIMetrics {
	return &APIMetrics{
		Method:        method,
		Endpoint:      endpoint,
		Service:       service,
		CorrelationID: correlationID,
		Timestamp:     time.Now(),
	}
}

// WithResponse adds response information to the metrics
func (m *APIMetrics) WithResponse(statusCode int, duration time.Duration) *APIMetrics {
	m.RequestsTotal = 1
	m.ResponseStatusCode = statusCode
	m.RequestDuration = duration
	return m
}

// WithError adds error information to the metrics
func (m *APIMetrics) WithError(errorType string, duration time.Duration) *APIMetrics {
	m.RequestsTotal = 1
	m.RequestDuration = duration
	m.ErrorType = errorType
	return m
}

// NewKafkaMetrics creates a new KafkaMetrics instance
func NewKafkaMetrics(topic string, partition int, offset int64, correlationID string) *KafkaMetrics {
	return &KafkaMetrics{
		MessagesConsumed: 1,
		Topic:            topic,
		Partition:        partition,
		Offset:           offset,
		CorrelationID:    correlationID,
		Timestamp:        time.Now(),
	}
}

// NewHealthMetrics creates a new HealthMetrics instance
func NewHealthMetrics(checkType string) *HealthMetrics {
	return &HealthMetrics{
		CheckType: checkType,
		Timestamp: time.Now(),
	}
}

// WithStatus adds status information to the health metrics
func (m *HealthMetrics) WithStatus(status string, duration time.Duration) *HealthMetrics {
	m.Status = status
	m.CheckDuration = duration
	return m
}

// WithDependency adds dependency information to the health metrics
func (m *HealthMetrics) WithDependency(name, status string) *HealthMetrics {
	m.DependencyName = name
	m.DependencyStatus = status
	return m
}
