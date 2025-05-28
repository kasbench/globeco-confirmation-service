package logger

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid json config",
			config: Config{
				Level:       "info",
				Format:      "json",
				Output:      "stdout",
				ServiceName: "test-service",
			},
			wantErr: false,
		},
		{
			name: "valid console config",
			config: Config{
				Level:       "debug",
				Format:      "console",
				Output:      "stderr",
				ServiceName: "test-service",
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			config: Config{
				Level:       "invalid",
				Format:      "json",
				Output:      "stdout",
				ServiceName: "test-service",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				assert.Equal(t, tt.config.ServiceName, logger.serviceName)
			}
		})
	}
}

func TestLogger_WithCorrelationID(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test-service",
	}

	logger, err := New(config)
	require.NoError(t, err)

	correlationID := "test-correlation-id"
	loggerWithCorr := logger.WithCorrelationID(correlationID)

	assert.NotNil(t, loggerWithCorr)
	assert.Equal(t, logger.serviceName, loggerWithCorr.serviceName)
}

func TestLogger_WithContext(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test-service",
	}

	logger, err := New(config)
	require.NoError(t, err)

	t.Run("context with correlation ID", func(t *testing.T) {
		correlationID := "test-correlation-id"
		ctx := WithCorrelationIDContext(context.Background(), correlationID)

		loggerWithCtx := logger.WithContext(ctx)
		assert.NotNil(t, loggerWithCtx)
	})

	t.Run("context without correlation ID", func(t *testing.T) {
		ctx := context.Background()

		loggerWithCtx := logger.WithContext(ctx)
		assert.NotNil(t, loggerWithCtx)
		assert.Equal(t, logger, loggerWithCtx) // Should return same logger
	})
}

func TestLogger_LogKafkaMessage(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test-service",
	}

	logger, err := New(config)
	require.NoError(t, err)

	ctx := WithCorrelationIDContext(context.Background(), "test-correlation")

	// This should not panic
	logger.LogKafkaMessage(ctx, "consume", "test-topic", 0, 123, 100*time.Millisecond)
}

func TestLogger_LogAPICall(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test-service",
	}

	logger, err := New(config)
	require.NoError(t, err)

	ctx := WithCorrelationIDContext(context.Background(), "test-correlation")

	t.Run("successful API call", func(t *testing.T) {
		// This should not panic
		logger.LogAPICall(ctx, "GET", "http://example.com", 200, 50*time.Millisecond, nil)
	})

	t.Run("failed API call", func(t *testing.T) {
		// This should not panic
		logger.LogAPICall(ctx, "POST", "http://example.com", 500, 100*time.Millisecond, assert.AnError)
	})
}

func TestLogger_LogError(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test-service",
	}

	logger, err := New(config)
	require.NoError(t, err)

	ctx := WithCorrelationIDContext(context.Background(), "test-correlation")

	// This should not panic
	logger.LogError(ctx, assert.AnError, "test error message")
}

func TestLogger_LogProcessingMetrics(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test-service",
	}

	logger, err := New(config)
	require.NoError(t, err)

	ctx := WithCorrelationIDContext(context.Background(), "test-correlation")

	// This should not panic
	logger.LogProcessingMetrics(ctx, 100, 95, 5, 50*time.Millisecond)
}

func TestGetCorrelationID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "context with correlation ID",
			ctx:      WithCorrelationIDContext(context.Background(), "test-id"),
			expected: "test-id",
		},
		{
			name:     "context without correlation ID",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name:     "context with wrong type",
			ctx:      context.WithValue(context.Background(), CorrelationIDKey, 123),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCorrelationID(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWithCorrelationIDContext(t *testing.T) {
	correlationID := "test-correlation-id"
	ctx := WithCorrelationIDContext(context.Background(), correlationID)

	retrievedID := GetCorrelationID(ctx)
	assert.Equal(t, correlationID, retrievedID)
}

func TestGenerateCorrelationID(t *testing.T) {
	// Generate multiple correlation IDs
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateCorrelationID()
		assert.NotEmpty(t, id)
		assert.False(t, ids[id], "Correlation ID should be unique: %s", id)
		ids[id] = true
	}
}

func TestLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run("level_"+level, func(t *testing.T) {
			config := Config{
				Level:       level,
				Format:      "json",
				Output:      "stdout",
				ServiceName: "test-service",
			}

			logger, err := New(config)
			assert.NoError(t, err)
			assert.NotNil(t, logger)
		})
	}
}

func TestLogFormats(t *testing.T) {
	formats := []string{"json", "console"}

	for _, format := range formats {
		t.Run("format_"+format, func(t *testing.T) {
			config := Config{
				Level:       "info",
				Format:      format,
				Output:      "stdout",
				ServiceName: "test-service",
			}

			logger, err := New(config)
			assert.NoError(t, err)
			assert.NotNil(t, logger)
		})
	}
}

func TestLogOutputs(t *testing.T) {
	outputs := []string{"stdout", "stderr"}

	for _, output := range outputs {
		t.Run("output_"+output, func(t *testing.T) {
			config := Config{
				Level:       "info",
				Format:      "json",
				Output:      output,
				ServiceName: "test-service",
			}

			logger, err := New(config)
			assert.NoError(t, err)
			assert.NotNil(t, logger)
		})
	}
}
