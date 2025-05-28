package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Loader handles configuration loading from various sources
type Loader struct {
	configName string
	configPath string
	envPrefix  string
}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	return &Loader{
		configName: "config",
		configPath: ".",
		envPrefix:  "CONFIRMATION",
	}
}

// WithConfigFile sets the config file name and path
func (l *Loader) WithConfigFile(name, path string) *Loader {
	l.configName = name
	l.configPath = path
	return l
}

// WithEnvPrefix sets the environment variable prefix
func (l *Loader) WithEnvPrefix(prefix string) *Loader {
	l.envPrefix = prefix
	return l
}

// Load loads configuration from files and environment variables
func (l *Loader) Load() (*Config, error) {
	// Start with defaults
	config := GetDefaults()

	// Setup Viper
	v := viper.New()

	// Set config file settings
	v.SetConfigName(l.configName)
	v.SetConfigType("yaml")
	v.AddConfigPath(l.configPath)
	v.AddConfigPath("/etc/confirmation-service/")
	v.AddConfigPath("$HOME/.confirmation-service")

	// Setup environment variable handling
	v.SetEnvPrefix(l.envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific environment variables for backward compatibility
	l.bindEnvironmentVariables(v)

	// Try to read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults + env vars
	}

	// Unmarshal into config struct
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Parse duration strings that might come from environment variables
	if err := l.parseDurations(v, config); err != nil {
		return nil, fmt.Errorf("failed to parse durations: %w", err)
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// bindEnvironmentVariables binds specific environment variables for backward compatibility
func (l *Loader) bindEnvironmentVariables(v *viper.Viper) {
	// HTTP configuration
	v.BindEnv("http.port", "HTTP_PORT", "PORT")
	v.BindEnv("http.host", "HTTP_HOST", "HOST")

	// Kafka configuration
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_TOPIC")
	v.BindEnv("kafka.consumer_group", "KAFKA_CONSUMER_GROUP")

	// Execution Service configuration
	v.BindEnv("execution_service.base_url", "EXECUTION_SERVICE_URL")
	v.BindEnv("execution_service.timeout", "EXECUTION_SERVICE_TIMEOUT")

	// Logging configuration
	v.BindEnv("logging.level", "LOG_LEVEL")
	v.BindEnv("logging.format", "LOG_FORMAT")
	v.BindEnv("logging.output", "LOG_OUTPUT")

	// Metrics configuration
	v.BindEnv("metrics.enabled", "METRICS_ENABLED")
	v.BindEnv("metrics.path", "METRICS_PATH")

	// Tracing configuration
	v.BindEnv("tracing.enabled", "TRACING_ENABLED")
	v.BindEnv("tracing.service_name", "TRACING_SERVICE_NAME")
	v.BindEnv("tracing.service_version", "TRACING_SERVICE_VERSION")
	v.BindEnv("tracing.exporter", "TRACING_EXPORTER")
}

// parseDurations handles duration parsing from string environment variables
func (l *Loader) parseDurations(v *viper.Viper, config *Config) error {
	durationFields := map[string]*time.Duration{
		"http.read_timeout":                         &config.HTTP.ReadTimeout,
		"http.write_timeout":                        &config.HTTP.WriteTimeout,
		"http.idle_timeout":                         &config.HTTP.IdleTimeout,
		"kafka.consumer_timeout":                    &config.Kafka.ConsumerTimeout,
		"kafka.retry_backoff":                       &config.Kafka.RetryBackoff,
		"execution_service.timeout":                 &config.ExecutionService.Timeout,
		"execution_service.retry_backoff":           &config.ExecutionService.RetryBackoff,
		"execution_service.circuit_breaker.timeout": &config.ExecutionService.CircuitBreaker.Timeout,
		"health.startup_grace_period":               &config.Health.StartupGracePeriod,
		"health.check_interval":                     &config.Health.CheckInterval,
	}

	for key, field := range durationFields {
		if v.IsSet(key) {
			if str := v.GetString(key); str != "" {
				if duration, err := time.ParseDuration(str); err == nil {
					*field = duration
				} else {
					return fmt.Errorf("invalid duration format for %s: %s", key, str)
				}
			}
		}
	}

	return nil
}

// LoadFromEnvironment loads configuration primarily from environment variables
func LoadFromEnvironment() (*Config, error) {
	loader := NewLoader()
	return loader.Load()
}

// LoadFromFile loads configuration from a specific file
func LoadFromFile(filePath string) (*Config, error) {
	// Extract directory and filename
	dir := "."
	name := "config"

	if filePath != "" {
		if lastSlash := strings.LastIndex(filePath, "/"); lastSlash != -1 {
			dir = filePath[:lastSlash]
			name = filePath[lastSlash+1:]
		} else {
			name = filePath
		}

		// Remove file extension if present
		if lastDot := strings.LastIndex(name, "."); lastDot != -1 {
			name = name[:lastDot]
		}
	}

	loader := NewLoader().WithConfigFile(name, dir)
	return loader.Load()
}

// GetEnvironment returns the current environment (dev, staging, prod)
func GetEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = os.Getenv("ENV")
	}
	if env == "" {
		env = "dev"
	}
	return env
}

// IsProduction returns true if running in production environment
func IsProduction() bool {
	env := GetEnvironment()
	return env == "prod" || env == "production"
}

// IsDevelopment returns true if running in development environment
func IsDevelopment() bool {
	env := GetEnvironment()
	return env == "dev" || env == "development"
}
