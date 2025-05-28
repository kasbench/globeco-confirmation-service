package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "enabled metrics",
			config: Config{
				Namespace: "test",
				Enabled:   true,
			},
		},
		{
			name: "disabled metrics",
			config: Config{
				Namespace: "test",
				Enabled:   false,
			},
		},
		{
			name: "default namespace",
			config: Config{
				Namespace: "",
				Enabled:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := New(tt.config)
			assert.NotNil(t, metrics)

			if tt.config.Enabled {
				// When enabled, metrics should be initialized
				assert.NotNil(t, metrics.MessagesProcessedTotal)
				assert.NotNil(t, metrics.MessagesFailedTotal)
				assert.NotNil(t, metrics.MessageProcessingTime)
			} else {
				// When disabled, metrics should be nil
				assert.Nil(t, metrics.MessagesProcessedTotal)
				assert.Nil(t, metrics.MessagesFailedTotal)
				assert.Nil(t, metrics.MessageProcessingTime)
			}
		})
	}
}

func TestMetrics_RecordMessageProcessed(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.RecordMessageProcessed()
		})
	}
}

func TestMetrics_RecordMessageFailed(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.RecordMessageFailed()
		})
	}
}

func TestMetrics_RecordMessageProcessingTime(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.RecordMessageProcessingTime(100 * time.Millisecond)
		})
	}
}

func TestMetrics_SetMessagesProcessing(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.SetMessagesProcessing(5.0)
		})
	}
}

func TestMetrics_RecordAPICall(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.RecordAPICall("GET", "/api/v1/test", "200", 50*time.Millisecond)
		})
	}
}

func TestMetrics_APICallsInFlight(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.IncAPICallsInFlight()
			metrics.DecAPICallsInFlight()
		})
	}
}

func TestMetrics_RecordKafkaMessage(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.RecordKafkaMessage()
		})
	}
}

func TestMetrics_SetKafkaConsumerLag(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.SetKafkaConsumerLag(100.0)
		})
	}
}

func TestMetrics_RecordKafkaConnectionError(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.RecordKafkaConnectionError()
		})
	}
}

func TestMetrics_CircuitBreaker(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.SetCircuitBreakerState("execution-service", 1.0)
			metrics.RecordCircuitBreakerOperation("execution-service", "success")
			metrics.RecordCircuitBreakerOperation("execution-service", "failure")
		})
	}
}

func TestMetrics_HealthCheck(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.SetHealthCheckStatus("kafka", true)
			metrics.SetHealthCheckStatus("execution-service", false)
			metrics.RecordHealthCheckDuration("kafka", 10*time.Millisecond)
		})
	}
}

func TestMetrics_SystemMetrics(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled metrics", true},
		{"disabled metrics", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Namespace: "test",
				Enabled:   tt.enabled,
			}
			metrics := New(config)

			// Should not panic regardless of enabled state
			metrics.SetActiveGoroutines(10.0)
			metrics.SetMemoryUsage(1024 * 1024 * 100) // 100MB
			metrics.SetCPUUsage(25.5)                 // 25.5%
		})
	}
}

func TestMetrics_AllMethods(t *testing.T) {
	// Test that all methods work together without panicking
	config := Config{
		Namespace: "integration_test",
		Enabled:   true,
	}
	metrics := New(config)

	// Message processing
	metrics.RecordMessageProcessed()
	metrics.RecordMessageFailed()
	metrics.RecordMessageProcessingTime(100 * time.Millisecond)
	metrics.SetMessagesProcessing(3.0)

	// API calls
	metrics.IncAPICallsInFlight()
	metrics.RecordAPICall("GET", "/api/v1/execution/123", "200", 50*time.Millisecond)
	metrics.DecAPICallsInFlight()

	// Kafka
	metrics.RecordKafkaMessage()
	metrics.SetKafkaConsumerLag(5.0)
	metrics.RecordKafkaConnectionError()

	// Circuit breaker
	metrics.SetCircuitBreakerState("execution-service", 0.0) // closed
	metrics.RecordCircuitBreakerOperation("execution-service", "success")

	// Health checks
	metrics.SetHealthCheckStatus("kafka", true)
	metrics.RecordHealthCheckDuration("kafka", 5*time.Millisecond)

	// System metrics
	metrics.SetActiveGoroutines(15.0)
	metrics.SetMemoryUsage(1024 * 1024 * 50) // 50MB
	metrics.SetCPUUsage(12.3)                // 12.3%
}
