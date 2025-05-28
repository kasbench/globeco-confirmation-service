package utils

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRetryer(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name           string
		config         RetryConfig
		expectedConfig RetryConfig
	}{
		{
			name: "default values applied",
			config: RetryConfig{
				MaxAttempts: 0,
			},
			expectedConfig: RetryConfig{
				MaxAttempts:   3,
				InitialDelay:  100 * time.Millisecond,
				MaxDelay:      5 * time.Second,
				BackoffFactor: 2.0,
			},
		},
		{
			name: "custom values preserved",
			config: RetryConfig{
				MaxAttempts:   5,
				InitialDelay:  200 * time.Millisecond,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
			expectedConfig: RetryConfig{
				MaxAttempts:   5,
				InitialDelay:  200 * time.Millisecond,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryer := NewRetryer(tt.config, appLogger)
			assert.NotNil(t, retryer)
			assert.Equal(t, tt.expectedConfig.MaxAttempts, retryer.config.MaxAttempts)
			assert.Equal(t, tt.expectedConfig.InitialDelay, retryer.config.InitialDelay)
			assert.Equal(t, tt.expectedConfig.MaxDelay, retryer.config.MaxDelay)
			assert.Equal(t, tt.expectedConfig.BackoffFactor, retryer.config.BackoffFactor)
		})
	}
}

func TestRetryer_Execute_Success(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	retryer := NewRetryer(config, appLogger)
	ctx := context.Background()

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		return nil // Success on first attempt
	}

	result := retryer.Execute(ctx, "test-operation", fn)

	assert.True(t, result.Success)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, callCount)
	assert.Nil(t, result.LastError)
	assert.Empty(t, result.ErrorHistory)
}

func TestRetryer_Execute_SuccessAfterRetry(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	retryer := NewRetryer(config, appLogger)
	ctx := context.Background()

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary failure")
		}
		return nil // Success on third attempt
	}

	result := retryer.Execute(ctx, "test-operation", fn)

	assert.True(t, result.Success)
	assert.Equal(t, 3, result.Attempts)
	assert.Equal(t, 3, callCount)
	assert.Nil(t, result.LastError)       // Should be nil on success
	assert.Len(t, result.ErrorHistory, 2) // Two failures before success
}

func TestRetryer_Execute_AllAttemptsFailed(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	retryer := NewRetryer(config, appLogger)
	ctx := context.Background()

	expectedError := errors.New("persistent failure")
	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		return expectedError
	}

	result := retryer.Execute(ctx, "test-operation", fn)

	assert.False(t, result.Success)
	assert.Equal(t, 3, result.Attempts)
	assert.Equal(t, 3, callCount)
	assert.Equal(t, expectedError, result.LastError)
	assert.Len(t, result.ErrorHistory, 3)
}

func TestRetryer_Execute_NonRetryableError(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	retryer := NewRetryer(config, appLogger)
	ctx := context.Background()

	// Create a non-retryable domain error
	nonRetryableError := domain.NewValidationError("invalid input", "field validation failed")
	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		return nonRetryableError
	}

	result := retryer.Execute(ctx, "test-operation", fn)

	assert.False(t, result.Success)
	assert.Equal(t, 1, result.Attempts) // Should stop after first attempt
	assert.Equal(t, 1, callCount)
	assert.Equal(t, nonRetryableError, result.LastError)
	assert.Len(t, result.ErrorHistory, 1)
}

func TestRetryer_Execute_ContextCancellation(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := RetryConfig{
		MaxAttempts:   5,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
	}

	retryer := NewRetryer(config, appLogger)
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		if callCount == 2 {
			cancel() // Cancel context after second attempt
		}
		return errors.New("failure")
	}

	result := retryer.Execute(ctx, "test-operation", fn)

	assert.False(t, result.Success)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, context.Canceled, result.LastError)
}

func TestRetryer_ExecuteWithStringResult(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := GetDefaultRetryConfig()
	retryer := NewRetryer(config, appLogger)
	ctx := context.Background()

	expectedResult := "success"
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		if callCount < 2 {
			return "", errors.New("temporary failure")
		}
		return expectedResult, nil
	}

	result, retryResult := retryer.ExecuteWithStringResult(ctx, "test-operation", fn)

	assert.Equal(t, expectedResult, result)
	assert.True(t, retryResult.Success)
	assert.Equal(t, 2, retryResult.Attempts)
	assert.Equal(t, 2, callCount)
}

func TestRetryer_ExecuteWithInterfaceResult(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := GetDefaultRetryConfig()
	retryer := NewRetryer(config, appLogger)
	ctx := context.Background()

	expectedResult := map[string]interface{}{"key": "value"}
	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		if callCount < 2 {
			return nil, errors.New("temporary failure")
		}
		return expectedResult, nil
	}

	result, retryResult := retryer.ExecuteWithInterfaceResult(ctx, "test-operation", fn)

	assert.Equal(t, expectedResult, result)
	assert.True(t, retryResult.Success)
	assert.Equal(t, 2, retryResult.Attempts)
	assert.Equal(t, 2, callCount)
}

func TestRetryer_calculateDelay(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := RetryConfig{
		MaxAttempts:   5,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
		JitterEnabled: false, // Disable jitter for predictable testing
	}

	retryer := NewRetryer(config, appLogger)

	tests := []struct {
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{1, 100 * time.Millisecond, 100 * time.Millisecond},
		{2, 200 * time.Millisecond, 200 * time.Millisecond},
		{3, 400 * time.Millisecond, 400 * time.Millisecond},
		{4, 800 * time.Millisecond, 800 * time.Millisecond},
		{5, 1 * time.Second, 1 * time.Second}, // Capped at MaxDelay
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := retryer.calculateDelay(tt.attempt)
			assert.GreaterOrEqual(t, delay, tt.expectedMin)
			assert.LessOrEqual(t, delay, tt.expectedMax)
		})
	}
}

func TestRetryer_calculateDelay_WithJitter(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
		JitterEnabled: true,
	}

	retryer := NewRetryer(config, appLogger)

	// Test that jitter produces different values
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = retryer.calculateDelay(2) // Second attempt
	}

	// Check that we get some variation (not all delays are identical)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "Jitter should produce different delay values")

	// Check that all delays are within reasonable bounds (±10% of 200ms)
	expectedBase := 200 * time.Millisecond
	minExpected := time.Duration(float64(expectedBase) * 0.9)
	maxExpected := time.Duration(float64(expectedBase) * 1.1)

	for _, delay := range delays {
		assert.GreaterOrEqual(t, delay, minExpected)
		assert.LessOrEqual(t, delay, maxExpected)
	}
}

func TestRetryer_isRetryableError(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := GetDefaultRetryConfig()
	retryer := NewRetryer(config, appLogger)

	tests := []struct {
		name      string
		error     error
		retryable bool
	}{
		{
			name:      "nil error",
			error:     nil,
			retryable: false,
		},
		{
			name:      "retryable domain error",
			error:     domain.NewExternalError("execution-service", "service unavailable", nil, true),
			retryable: true,
		},
		{
			name:      "non-retryable domain error",
			error:     domain.NewValidationError("invalid input", "field validation failed"),
			retryable: false,
		},
		{
			name:      "timeout error",
			error:     errors.New("timeout occurred"),
			retryable: true,
		},
		{
			name:      "connection refused error",
			error:     errors.New("connection refused"),
			retryable: true,
		},
		{
			name:      "service unavailable error",
			error:     errors.New("service unavailable"),
			retryable: true,
		},
		{
			name:      "generic error",
			error:     errors.New("some other error"),
			retryable: true, // Changed to true since we default to retryable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := retryer.isRetryableError(tt.error)
			assert.Equal(t, tt.retryable, result)
		})
	}
}

func TestGetDefaultRetryConfig(t *testing.T) {
	config := GetDefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, config.InitialDelay)
	assert.Equal(t, 5*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffFactor)
	assert.True(t, config.JitterEnabled)
	assert.Contains(t, config.RetryableErrors, "*domain.ExternalError")
	assert.Contains(t, config.RetryableErrors, "*domain.TimeoutError")
}
