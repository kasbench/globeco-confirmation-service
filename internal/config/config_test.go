package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaults(t *testing.T) {
	config := GetDefaults()

	// Test HTTP defaults
	assert.Equal(t, 8086, config.HTTP.Port)
	assert.Equal(t, "0.0.0.0", config.HTTP.Host)
	assert.Equal(t, 30*time.Second, config.HTTP.ReadTimeout)
	assert.Equal(t, 30*time.Second, config.HTTP.WriteTimeout)
	assert.Equal(t, 60*time.Second, config.HTTP.IdleTimeout)

	// Test Kafka defaults
	assert.Equal(t, []string{"globeco-execution-service-kafka:9092"}, config.Kafka.Brokers)
	assert.Equal(t, "fills", config.Kafka.Topic)
	assert.Equal(t, "confirmation-service", config.Kafka.ConsumerGroup)
	assert.Equal(t, 30*time.Second, config.Kafka.ConsumerTimeout)
	assert.Equal(t, 3, config.Kafka.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, config.Kafka.RetryBackoff)

	// Test Execution Service defaults
	assert.Equal(t, "http://globeco-execution-service:8084", config.ExecutionService.BaseURL)
	assert.Equal(t, 10*time.Second, config.ExecutionService.Timeout)
	assert.Equal(t, 3, config.ExecutionService.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, config.ExecutionService.RetryBackoff)
	assert.Equal(t, 5, config.ExecutionService.CircuitBreaker.FailureThreshold)
	assert.Equal(t, 30*time.Second, config.ExecutionService.CircuitBreaker.Timeout)

	// Test Logging defaults
	assert.Equal(t, "info", config.Logging.Level)
	assert.Equal(t, "json", config.Logging.Format)
	assert.Equal(t, "stdout", config.Logging.Output)

	// Test Metrics defaults
	assert.True(t, config.Metrics.Enabled)
	assert.Equal(t, "/metrics", config.Metrics.Path)
	assert.Equal(t, "confirmation", config.Metrics.Namespace)

	// Test Tracing defaults
	assert.True(t, config.Tracing.Enabled)
	assert.Equal(t, "confirmation-service", config.Tracing.ServiceName)
	assert.Equal(t, "1.0.0", config.Tracing.ServiceVersion)
	assert.Equal(t, "stdout", config.Tracing.Exporter)

	// Test Performance defaults
	assert.Equal(t, 10, config.Performance.MaxConcurrentRequests)
	assert.Equal(t, 1000, config.Performance.MessageBufferSize)
	assert.Equal(t, 5, config.Performance.WorkerPoolSize)

	// Test Health defaults
	assert.Equal(t, 30*time.Second, config.Health.StartupGracePeriod)
	assert.Equal(t, 10*time.Second, config.Health.CheckInterval)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid default config",
			config:  GetDefaults(),
			wantErr: false,
		},
		{
			name: "invalid HTTP port - too low",
			config: func() *Config {
				c := GetDefaults()
				c.HTTP.Port = 0
				return c
			}(),
			wantErr: true,
			errMsg:  "http.port must be between 1 and 65535",
		},
		{
			name: "invalid HTTP port - too high",
			config: func() *Config {
				c := GetDefaults()
				c.HTTP.Port = 70000
				return c
			}(),
			wantErr: true,
			errMsg:  "http.port must be between 1 and 65535",
		},
		{
			name: "empty HTTP host",
			config: func() *Config {
				c := GetDefaults()
				c.HTTP.Host = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "http.host is required",
		},
		{
			name: "empty Kafka brokers",
			config: func() *Config {
				c := GetDefaults()
				c.Kafka.Brokers = []string{}
				return c
			}(),
			wantErr: true,
			errMsg:  "kafka.brokers is required",
		},
		{
			name: "empty Kafka topic",
			config: func() *Config {
				c := GetDefaults()
				c.Kafka.Topic = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "kafka.topic is required",
		},
		{
			name: "empty Kafka consumer group",
			config: func() *Config {
				c := GetDefaults()
				c.Kafka.ConsumerGroup = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "kafka.consumer_group is required",
		},
		{
			name: "empty execution service base URL",
			config: func() *Config {
				c := GetDefaults()
				c.ExecutionService.BaseURL = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "execution_service.base_url is required",
		},
		{
			name: "invalid circuit breaker failure threshold",
			config: func() *Config {
				c := GetDefaults()
				c.ExecutionService.CircuitBreaker.FailureThreshold = 0
				return c
			}(),
			wantErr: true,
			errMsg:  "execution_service.circuit_breaker.failure_threshold must be at least 1",
		},
		{
			name: "invalid logging level",
			config: func() *Config {
				c := GetDefaults()
				c.Logging.Level = "invalid"
				return c
			}(),
			wantErr: true,
			errMsg:  "logging.level must be one of: debug, info, warn, error",
		},
		{
			name: "invalid logging format",
			config: func() *Config {
				c := GetDefaults()
				c.Logging.Format = "invalid"
				return c
			}(),
			wantErr: true,
			errMsg:  "logging.format must be one of: json, console",
		},
		{
			name: "invalid logging output",
			config: func() *Config {
				c := GetDefaults()
				c.Logging.Output = "invalid"
				return c
			}(),
			wantErr: true,
			errMsg:  "logging.output must be one of: stdout, stderr, file",
		},
		{
			name: "invalid tracing exporter",
			config: func() *Config {
				c := GetDefaults()
				c.Tracing.Exporter = "invalid"
				return c
			}(),
			wantErr: true,
			errMsg:  "tracing.exporter must be one of: stdout, jaeger, otlp",
		},
		{
			name: "invalid max concurrent requests",
			config: func() *Config {
				c := GetDefaults()
				c.Performance.MaxConcurrentRequests = 0
				return c
			}(),
			wantErr: true,
			errMsg:  "performance.max_concurrent_requests must be at least 1",
		},
		{
			name: "invalid message buffer size",
			config: func() *Config {
				c := GetDefaults()
				c.Performance.MessageBufferSize = 0
				return c
			}(),
			wantErr: true,
			errMsg:  "performance.message_buffer_size must be at least 1",
		},
		{
			name: "invalid worker pool size",
			config: func() *Config {
				c := GetDefaults()
				c.Performance.WorkerPoolSize = 0
				return c
			}(),
			wantErr: true,
			errMsg:  "performance.worker_pool_size must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetHTTPAddress(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "default configuration",
			host:     "0.0.0.0",
			port:     8086,
			expected: "0.0.0.0:8086",
		},
		{
			name:     "localhost configuration",
			host:     "localhost",
			port:     3000,
			expected: "localhost:3000",
		},
		{
			name:     "specific IP configuration",
			host:     "192.168.1.100",
			port:     8080,
			expected: "192.168.1.100:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := GetDefaults()
			config.HTTP.Host = tt.host
			config.HTTP.Port = tt.port

			address := config.GetHTTPAddress()
			assert.Equal(t, tt.expected, address)
		})
	}
}

func TestValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run("valid_level_"+level, func(t *testing.T) {
			config := GetDefaults()
			config.Logging.Level = level

			err := config.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestValidLogFormats(t *testing.T) {
	validFormats := []string{"json", "console"}

	for _, format := range validFormats {
		t.Run("valid_format_"+format, func(t *testing.T) {
			config := GetDefaults()
			config.Logging.Format = format

			err := config.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestValidLogOutputs(t *testing.T) {
	validOutputs := []string{"stdout", "stderr", "file"}

	for _, output := range validOutputs {
		t.Run("valid_output_"+output, func(t *testing.T) {
			config := GetDefaults()
			config.Logging.Output = output

			err := config.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestValidTracingExporters(t *testing.T) {
	validExporters := []string{"stdout", "jaeger", "otlp"}

	for _, exporter := range validExporters {
		t.Run("valid_exporter_"+exporter, func(t *testing.T) {
			config := GetDefaults()
			config.Tracing.Exporter = exporter

			err := config.Validate()
			assert.NoError(t, err)
		})
	}
}
