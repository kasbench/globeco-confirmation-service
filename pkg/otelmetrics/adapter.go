package otelmetrics

import (
	"context"
	"time"

	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
)

// Adapter wraps OpenTelemetry metrics to implement the existing metrics interface
// This allows gradual migration from Prometheus to OpenTelemetry metrics
type Adapter struct {
	otelMetrics *Metrics
	promMetrics *metrics.Metrics
	ctx         context.Context
}

// NewAdapter creates a new adapter that uses both OpenTelemetry and Prometheus metrics
func NewAdapter(otelMetrics *Metrics, promMetrics *metrics.Metrics) *Adapter {
	return &Adapter{
		otelMetrics: otelMetrics,
		promMetrics: promMetrics,
		ctx:         context.Background(),
	}
}

// RecordMessageProcessed records a processed message in both systems
func (a *Adapter) RecordMessageProcessed() {
	if a.promMetrics != nil {
		a.promMetrics.RecordMessageProcessed()
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordMessageProcessed(a.ctx)
	}
}

// RecordMessageFailed records a failed message in both systems
func (a *Adapter) RecordMessageFailed() {
	if a.promMetrics != nil {
		a.promMetrics.RecordMessageFailed()
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordMessageFailed(a.ctx)
	}
}

// RecordMessageProcessingTime records message processing time in both systems
func (a *Adapter) RecordMessageProcessingTime(duration time.Duration) {
	if a.promMetrics != nil {
		a.promMetrics.RecordMessageProcessingTime(duration)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordMessageProcessingTime(a.ctx, duration)
	}
}



// RecordAPICall records an API call in both systems
func (a *Adapter) RecordAPICall(method, endpoint, statusCode string, duration time.Duration) {
	if a.promMetrics != nil {
		a.promMetrics.RecordAPICall(method, endpoint, statusCode, duration)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordAPICall(a.ctx, method, endpoint, statusCode, duration)
	}
}

// IncAPICallsInFlight increments in-flight API calls in both systems
func (a *Adapter) IncAPICallsInFlight() {
	if a.promMetrics != nil {
		a.promMetrics.IncAPICallsInFlight()
	}
	if a.otelMetrics != nil {
		a.otelMetrics.IncAPICallsInFlight(a.ctx)
	}
}

// DecAPICallsInFlight decrements in-flight API calls in both systems
func (a *Adapter) DecAPICallsInFlight() {
	if a.promMetrics != nil {
		a.promMetrics.DecAPICallsInFlight()
	}
	if a.otelMetrics != nil {
		a.otelMetrics.DecAPICallsInFlight(a.ctx)
	}
}

// RecordKafkaMessage records a Kafka message in both systems
func (a *Adapter) RecordKafkaMessage() {
	if a.promMetrics != nil {
		a.promMetrics.RecordKafkaMessage()
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordKafkaMessage(a.ctx)
	}
}

// SetKafkaConsumerLag sets Kafka consumer lag in both systems
func (a *Adapter) SetKafkaConsumerLag(lag float64) {
	if a.promMetrics != nil {
		a.promMetrics.SetKafkaConsumerLag(lag)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.SetKafkaConsumerLag(a.ctx, lag)
	}
}

// RecordKafkaConnectionError records a Kafka connection error in both systems
func (a *Adapter) RecordKafkaConnectionError() {
	if a.promMetrics != nil {
		a.promMetrics.RecordKafkaConnectionError()
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordKafkaConnectionError(a.ctx)
	}
}

// SetCircuitBreakerState sets circuit breaker state in both systems
func (a *Adapter) SetCircuitBreakerState(name string, state float64) {
	if a.promMetrics != nil {
		a.promMetrics.SetCircuitBreakerState(name, state)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.SetCircuitBreakerState(a.ctx, name, int64(state))
	}
}

// RecordCircuitBreakerOperation records a circuit breaker operation in both systems
func (a *Adapter) RecordCircuitBreakerOperation(name, result string) {
	if a.promMetrics != nil {
		a.promMetrics.RecordCircuitBreakerOperation(name, result)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordCircuitBreakerOperation(a.ctx, name, result)
	}
}

// SetHealthCheckStatus sets health check status in both systems
func (a *Adapter) SetHealthCheckStatus(checkName string, healthy bool) {
	if a.promMetrics != nil {
		a.promMetrics.SetHealthCheckStatus(checkName, healthy)
	}
	if a.otelMetrics != nil {
		status := int64(0)
		if healthy {
			status = 1
		}
		a.otelMetrics.SetHealthCheckStatus(a.ctx, checkName, status)
	}
}

// RecordHealthCheckDuration records health check duration in both systems
func (a *Adapter) RecordHealthCheckDuration(checkName string, duration time.Duration) {
	if a.promMetrics != nil {
		a.promMetrics.RecordHealthCheckDuration(checkName, duration)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.RecordHealthCheckDuration(a.ctx, checkName, duration)
	}
}

// SetMessagesProcessing sets the current number of messages being processed
func (a *Adapter) SetMessagesProcessing(count float64) {
	if a.promMetrics != nil {
		a.promMetrics.SetMessagesProcessing(count)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.SetMessagesProcessingCurrent(a.ctx, int64(count))
	}
}

// RecordHealthCheck records both health check status and duration in both systems
func (a *Adapter) RecordHealthCheck(checkName string, healthy bool, duration time.Duration) {
	if a.promMetrics != nil {
		a.promMetrics.RecordHealthCheck(checkName, healthy, duration)
	}
	if a.otelMetrics != nil {
		status := int64(0)
		if healthy {
			status = 1
		}
		a.otelMetrics.SetHealthCheckStatus(a.ctx, checkName, status)
		a.otelMetrics.RecordHealthCheckDuration(a.ctx, checkName, duration)
	}
}

// SetActiveGoroutines sets the number of active goroutines in both systems
func (a *Adapter) SetActiveGoroutines(count float64) {
	if a.promMetrics != nil {
		a.promMetrics.SetActiveGoroutines(count)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.SetActiveGoroutines(a.ctx, int64(count))
	}
}

// SetMemoryUsage sets the current memory usage in both systems
func (a *Adapter) SetMemoryUsage(bytes float64) {
	if a.promMetrics != nil {
		a.promMetrics.SetMemoryUsage(bytes)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.SetMemoryUsage(a.ctx, int64(bytes))
	}
}

// SetCPUUsage sets the current CPU usage percentage in both systems
func (a *Adapter) SetCPUUsage(percent float64) {
	if a.promMetrics != nil {
		a.promMetrics.SetCPUUsage(percent)
	}
	if a.otelMetrics != nil {
		a.otelMetrics.SetCPUUsage(a.ctx, percent)
	}
}

// UpdateSystemMetrics updates system metrics (OpenTelemetry only)
func (a *Adapter) UpdateSystemMetrics() {
	if a.otelMetrics != nil {
		a.otelMetrics.UpdateSystemMetrics(a.ctx)
	}
}

// IsEnabled returns whether metrics are enabled
func (a *Adapter) IsEnabled() bool {
	return (a.promMetrics != nil) || (a.otelMetrics != nil && a.otelMetrics.IsEnabled())
}