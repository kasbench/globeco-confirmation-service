package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader()

	assert.Equal(t, "config", loader.configName)
	assert.Equal(t, ".", loader.configPath)
	assert.Equal(t, "CONFIRMATION", loader.envPrefix)
}

func TestLoader_WithConfigFile(t *testing.T) {
	loader := NewLoader().WithConfigFile("test-config", "/etc/test")

	assert.Equal(t, "test-config", loader.configName)
	assert.Equal(t, "/etc/test", loader.configPath)
}

func TestLoader_WithEnvPrefix(t *testing.T) {
	loader := NewLoader().WithEnvPrefix("TEST")

	assert.Equal(t, "TEST", loader.envPrefix)
}

func TestLoadFromEnvironment(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"HTTP_PORT", "KAFKA_BROKERS", "EXECUTION_SERVICE_URL", "LOG_LEVEL",
	}

	for _, env := range envVars {
		originalEnv[env] = os.Getenv(env)
	}

	// Clean up after test
	defer func() {
		for _, env := range envVars {
			if val, exists := originalEnv[env]; exists {
				os.Setenv(env, val)
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	// Set test environment variables
	os.Setenv("HTTP_PORT", "9090")
	os.Setenv("KAFKA_BROKERS", "localhost:9092,localhost:9092")
	os.Setenv("EXECUTION_SERVICE_URL", "http://localhost:8085")
	os.Setenv("LOG_LEVEL", "debug")

	config, err := LoadFromEnvironment()
	require.NoError(t, err)

	// Verify environment variables were applied
	assert.Equal(t, 9090, config.HTTP.Port)
	assert.Equal(t, "debug", config.Logging.Level)
	assert.Equal(t, "http://localhost:8085", config.ExecutionService.BaseURL)
}

func TestLoadFromFile_NonExistentFile(t *testing.T) {
	config, err := LoadFromFile("non-existent-config.yaml")

	// Should not error, should return defaults
	require.NoError(t, err)
	assert.NotNil(t, config)

	// Should have default values
	assert.Equal(t, 8086, config.HTTP.Port)
	assert.Equal(t, "fills", config.Kafka.Topic)
}

func TestGetEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		envValue    string
		altEnvVar   string
		altEnvValue string
		expected    string
	}{
		{
			name:     "ENVIRONMENT variable set",
			envVar:   "ENVIRONMENT",
			envValue: "production",
			expected: "production",
		},
		{
			name:        "ENV variable set when ENVIRONMENT not set",
			altEnvVar:   "ENV",
			altEnvValue: "staging",
			expected:    "staging",
		},
		{
			name:     "default when no environment variables set",
			expected: "dev",
		},
		{
			name:        "ENVIRONMENT takes precedence over ENV",
			envVar:      "ENVIRONMENT",
			envValue:    "prod",
			altEnvVar:   "ENV",
			altEnvValue: "staging",
			expected:    "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original values
			originalEnv := os.Getenv("ENVIRONMENT")
			originalAltEnv := os.Getenv("ENV")

			// Clean up after test
			defer func() {
				if originalEnv != "" {
					os.Setenv("ENVIRONMENT", originalEnv)
				} else {
					os.Unsetenv("ENVIRONMENT")
				}
				if originalAltEnv != "" {
					os.Setenv("ENV", originalAltEnv)
				} else {
					os.Unsetenv("ENV")
				}
			}()

			// Clear environment variables
			os.Unsetenv("ENVIRONMENT")
			os.Unsetenv("ENV")

			// Set test values
			if tt.envVar != "" {
				os.Setenv(tt.envVar, tt.envValue)
			}
			if tt.altEnvVar != "" {
				os.Setenv(tt.altEnvVar, tt.altEnvValue)
			}

			result := GetEnvironment()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsProduction(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"prod environment", "prod", true},
		{"production environment", "production", true},
		{"dev environment", "dev", false},
		{"staging environment", "staging", false},
		{"empty environment", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("ENVIRONMENT")
			defer func() {
				if original != "" {
					os.Setenv("ENVIRONMENT", original)
				} else {
					os.Unsetenv("ENVIRONMENT")
				}
			}()

			// Set test value
			if tt.envValue != "" {
				os.Setenv("ENVIRONMENT", tt.envValue)
			} else {
				os.Unsetenv("ENVIRONMENT")
			}

			result := IsProduction()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDevelopment(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"dev environment", "dev", true},
		{"development environment", "development", true},
		{"prod environment", "prod", false},
		{"staging environment", "staging", false},
		{"empty environment (defaults to dev)", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("ENVIRONMENT")
			defer func() {
				if original != "" {
					os.Setenv("ENVIRONMENT", original)
				} else {
					os.Unsetenv("ENVIRONMENT")
				}
			}()

			// Set test value
			if tt.envValue != "" {
				os.Setenv("ENVIRONMENT", tt.envValue)
			} else {
				os.Unsetenv("ENVIRONMENT")
			}

			result := IsDevelopment()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDurationParsing(t *testing.T) {
	// Save original environment
	originalTimeout := os.Getenv("EXECUTION_SERVICE_TIMEOUT")
	defer func() {
		if originalTimeout != "" {
			os.Setenv("EXECUTION_SERVICE_TIMEOUT", originalTimeout)
		} else {
			os.Unsetenv("EXECUTION_SERVICE_TIMEOUT")
		}
	}()

	// Set duration environment variable
	os.Setenv("EXECUTION_SERVICE_TIMEOUT", "5s")

	config, err := LoadFromEnvironment()
	require.NoError(t, err)

	// Verify duration was parsed correctly
	assert.Equal(t, 5*time.Second, config.ExecutionService.Timeout)
}

func TestInvalidDurationParsing(t *testing.T) {
	// Save original environment
	originalTimeout := os.Getenv("EXECUTION_SERVICE_TIMEOUT")
	defer func() {
		if originalTimeout != "" {
			os.Setenv("EXECUTION_SERVICE_TIMEOUT", originalTimeout)
		} else {
			os.Unsetenv("EXECUTION_SERVICE_TIMEOUT")
		}
	}()

	// Set invalid duration environment variable
	os.Setenv("EXECUTION_SERVICE_TIMEOUT", "invalid-duration")

	_, err := LoadFromEnvironment()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration")
}

func TestConfigValidationFailure(t *testing.T) {
	// Save original environment
	originalPort := os.Getenv("HTTP_PORT")
	defer func() {
		if originalPort != "" {
			os.Setenv("HTTP_PORT", originalPort)
		} else {
			os.Unsetenv("HTTP_PORT")
		}
	}()

	// Set invalid port
	os.Setenv("HTTP_PORT", "70000")

	_, err := LoadFromEnvironment()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration validation failed")
}
