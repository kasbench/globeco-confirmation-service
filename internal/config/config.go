package config

import (
	"fmt"
	"time"
)

// Config represents the application configuration
type Config struct {
	HTTP             HTTPConfig             `mapstructure:"http"`
	Kafka            KafkaConfig            `mapstructure:"kafka"`
	ExecutionService ExecutionServiceConfig `mapstructure:"execution_service"`
	Logging          LoggingConfig          `mapstructure:"logging"`
	Metrics          MetricsConfig          `mapstructure:"metrics"`
	Tracing          TracingConfig          `mapstructure:"tracing"`
	Performance      PerformanceConfig      `mapstructure:"performance"`
	Health           HealthConfig           `mapstructure:"health"`
}

// HTTPConfig represents HTTP server configuration
type HTTPConfig struct {
	Port         int           `mapstructure:"port" validate:"required,min=1,max=65535"`
	Host         string        `mapstructure:"host" validate:"required"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" validate:"required"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" validate:"required"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout" validate:"required"`
}

// KafkaConfig represents Kafka configuration
type KafkaConfig struct {
	Brokers           []string      `mapstructure:"brokers" validate:"required,min=1"`
	Topic             string        `mapstructure:"topic" validate:"required"`
	ConsumerGroup     string        `mapstructure:"consumer_group" validate:"required"`
	ConsumerTimeout   time.Duration `mapstructure:"consumer_timeout" validate:"required"`
	ConnectionTimeout time.Duration `mapstructure:"connection_timeout" validate:"required"`
	FetchTimeout      time.Duration `mapstructure:"fetch_timeout" validate:"required"`
	MaxRetries        int           `mapstructure:"max_retries" validate:"required,min=0"`
	RetryBackoff      time.Duration `mapstructure:"retry_backoff" validate:"required"`
}

// ExecutionServiceConfig represents Execution Service configuration
type ExecutionServiceConfig struct {
	BaseURL        string               `mapstructure:"base_url" validate:"required,url"`
	Timeout        time.Duration        `mapstructure:"timeout" validate:"required"`
	MaxRetries     int                  `mapstructure:"max_retries" validate:"required,min=0"`
	RetryBackoff   time.Duration        `mapstructure:"retry_backoff" validate:"required"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
}

// CircuitBreakerConfig represents circuit breaker configuration
type CircuitBreakerConfig struct {
	FailureThreshold int           `mapstructure:"failure_threshold" validate:"required,min=1"`
	Timeout          time.Duration `mapstructure:"timeout" validate:"required"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level" validate:"required,oneof=debug info warn error"`
	Format string `mapstructure:"format" validate:"required,oneof=json console"`
	Output string `mapstructure:"output" validate:"required,oneof=stdout stderr file"`
}

// MetricsConfig represents metrics configuration
type MetricsConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Path      string `mapstructure:"path" validate:"required"`
	Namespace string `mapstructure:"namespace" validate:"required"`
}

// TracingConfig represents tracing configuration
type TracingConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	ServiceName    string `mapstructure:"service_name" validate:"required"`
	ServiceVersion string `mapstructure:"service_version" validate:"required"`
	Exporter       string `mapstructure:"exporter" validate:"required,oneof=stdout jaeger otlp"`
}

// PerformanceConfig represents performance configuration
type PerformanceConfig struct {
	MaxConcurrentRequests int `mapstructure:"max_concurrent_requests" validate:"required,min=1"`
	MessageBufferSize     int `mapstructure:"message_buffer_size" validate:"required,min=1"`
	WorkerPoolSize        int `mapstructure:"worker_pool_size" validate:"required,min=1"`
}

// HealthConfig represents health check configuration
type HealthConfig struct {
	StartupGracePeriod time.Duration `mapstructure:"startup_grace_period" validate:"required"`
	CheckInterval      time.Duration `mapstructure:"check_interval" validate:"required"`
}

// GetDefaults returns a Config with default values
func GetDefaults() *Config {
	return &Config{
		HTTP: HTTPConfig{
			Port:         8086,
			Host:         "0.0.0.0",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Kafka: KafkaConfig{
			Brokers:           []string{"globeco-execution-service-kafka:9093"},
			Topic:             "fills",
			ConsumerGroup:     "confirmation-service",
			ConsumerTimeout:   30 * time.Second,
			ConnectionTimeout: 10 * time.Second,
			FetchTimeout:      5 * time.Second,
			MaxRetries:        3,
			RetryBackoff:      100 * time.Millisecond,
		},
		ExecutionService: ExecutionServiceConfig{
			BaseURL:      "http://globeco-execution-service:8084",
			Timeout:      10 * time.Second,
			MaxRetries:   3,
			RetryBackoff: 100 * time.Millisecond,
			CircuitBreaker: CircuitBreakerConfig{
				FailureThreshold: 5,
				Timeout:          30 * time.Second,
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Metrics: MetricsConfig{
			Enabled:   true,
			Path:      "/metrics",
			Namespace: "confirmation",
		},
		Tracing: TracingConfig{
			Enabled:        true,
			ServiceName:    "confirmation-service",
			ServiceVersion: "1.0.0",
			Exporter:       "stdout",
		},
		Performance: PerformanceConfig{
			MaxConcurrentRequests: 10,
			MessageBufferSize:     1000,
			WorkerPoolSize:        5,
		},
		Health: HealthConfig{
			StartupGracePeriod: 30 * time.Second,
			CheckInterval:      10 * time.Second,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate HTTP configuration
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535, got %d", c.HTTP.Port)
	}

	if c.HTTP.Host == "" {
		return fmt.Errorf("http.host is required")
	}

	// Validate Kafka configuration
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers is required")
	}

	if c.Kafka.Topic == "" {
		return fmt.Errorf("kafka.topic is required")
	}

	if c.Kafka.ConsumerGroup == "" {
		return fmt.Errorf("kafka.consumer_group is required")
	}

	// Validate Execution Service configuration
	if c.ExecutionService.BaseURL == "" {
		return fmt.Errorf("execution_service.base_url is required")
	}

	if c.ExecutionService.CircuitBreaker.FailureThreshold < 1 {
		return fmt.Errorf("execution_service.circuit_breaker.failure_threshold must be at least 1")
	}

	// Validate Logging configuration
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}

	validLogFormats := map[string]bool{"json": true, "console": true}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("logging.format must be one of: json, console")
	}

	validLogOutputs := map[string]bool{"stdout": true, "stderr": true, "file": true}
	if !validLogOutputs[c.Logging.Output] {
		return fmt.Errorf("logging.output must be one of: stdout, stderr, file")
	}

	// Validate Tracing configuration
	validTracingExporters := map[string]bool{"stdout": true, "jaeger": true, "otlp": true}
	if !validTracingExporters[c.Tracing.Exporter] {
		return fmt.Errorf("tracing.exporter must be one of: stdout, jaeger, otlp")
	}

	// Validate Performance configuration
	if c.Performance.MaxConcurrentRequests < 1 {
		return fmt.Errorf("performance.max_concurrent_requests must be at least 1")
	}

	if c.Performance.MessageBufferSize < 1 {
		return fmt.Errorf("performance.message_buffer_size must be at least 1")
	}

	if c.Performance.WorkerPoolSize < 1 {
		return fmt.Errorf("performance.worker_pool_size must be at least 1")
	}

	return nil
}

// GetHTTPAddress returns the HTTP server address
func (c *Config) GetHTTPAddress() string {
	return fmt.Sprintf("%s:%d", c.HTTP.Host, c.HTTP.Port)
}
