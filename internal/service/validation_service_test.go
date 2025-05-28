package service

import (
	"context"
	"testing"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidationService(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := ValidationConfig{
		Logger: appLogger,
	}

	service := NewValidationService(config)

	assert.NotNil(t, service)
	assert.Equal(t, appLogger, service.logger)
}

func TestValidationService_ValidateFillMessage_ValidMessage(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewValidationService(ValidationConfig{Logger: appLogger})
	ctx := context.Background()

	// Create a valid fill message
	fill := &domain.Fill{
		ID:                  123,
		ExecutionServiceID:  456,
		IsOpen:              false,
		ExecutionStatus:     "FULL",
		TradeType:           "BUY",
		Destination:         "ML",
		SecurityID:          "SEC123",
		Ticker:              "IBM",
		Quantity:            1000,
		ReceivedTimestamp:   float64(time.Now().Unix() - 3600), // 1 hour ago
		SentTimestamp:       float64(time.Now().Unix() - 3500), // 55 minutes ago
		LastFilledTimestamp: float64(time.Now().Unix() - 3400), // 50 minutes ago
		QuantityFilled:      1000,
		AveragePrice:        190.41,
		NumberOfFills:       3,
		TotalAmount:         190410.0,
		Version:             1,
	}

	result := service.ValidateFillMessage(ctx, fill)

	assert.True(t, result.IsValid)
	assert.Empty(t, result.Errors)
	// May have warnings for format validation
}

func TestValidationService_ValidateFillMessage_RequiredFields(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewValidationService(ValidationConfig{Logger: appLogger})
	ctx := context.Background()

	tests := []struct {
		name          string
		fill          *domain.Fill
		expectedError string
	}{
		{
			name: "missing execution service ID",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 0, // Invalid
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "executionServiceId must be a positive integer",
		},
		{
			name: "negative quantity filled",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     -100, // Invalid
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "quantityFilled must be non-negative",
		},
		{
			name: "zero average price",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       0, // Invalid
				Version:            1,
			},
			expectedError: "averagePrice must be positive",
		},
		{
			name: "empty execution status",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "", // Invalid
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "executionStatus is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ValidateFillMessage(ctx, tt.fill)

			assert.False(t, result.IsValid)
			assert.NotEmpty(t, result.Errors)

			found := false
			for _, err := range result.Errors {
				if err.Message == tt.expectedError {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected error message not found: %s", tt.expectedError)
		})
	}
}

func TestValidationService_ValidateFillMessage_BusinessRules(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewValidationService(ValidationConfig{Logger: appLogger})
	ctx := context.Background()

	tests := []struct {
		name          string
		fill          *domain.Fill
		expectedError string
		isWarning     bool
	}{
		{
			name: "quantity filled exceeds total",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1500, // Exceeds total
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "quantityFilled (1500) cannot exceed original quantity (1000)",
			isWarning:     false,
		},
		{
			name: "invalid execution status",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "INVALID", // Invalid status
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "executionStatus 'INVALID' is not valid. Must be one of: PENDING, PARTIAL, FULL, CANCELLED",
			isWarning:     false,
		},
		{
			name: "invalid trade type",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "INVALID", // Invalid trade type
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "tradeType 'INVALID' is not valid. Must be BUY or SELL",
			isWarning:     false,
		},
		{
			name: "high average price warning",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       15000, // Very high price
				Version:            1,
			},
			expectedError: "averagePrice (15000.00) is unusually high",
			isWarning:     true,
		},
		{
			name: "status quantity mismatch warning",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     500, // Should be 1000 for FULL status
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "execution status is FULL but quantityFilled (500) does not equal total quantity (1000)",
			isWarning:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ValidateFillMessage(ctx, tt.fill)

			if tt.isWarning {
				// Should be valid but have warnings
				assert.True(t, result.IsValid)
				assert.NotEmpty(t, result.Warnings)

				found := false
				for _, warning := range result.Warnings {
					if warning.Message == tt.expectedError {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected warning message not found: %s", tt.expectedError)
			} else {
				// Should be invalid with errors
				assert.False(t, result.IsValid)
				assert.NotEmpty(t, result.Errors)

				found := false
				for _, err := range result.Errors {
					if err.Message == tt.expectedError {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error message not found: %s", tt.expectedError)
			}
		})
	}
}

func TestValidationService_ValidateFillMessage_TimestampValidation(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewValidationService(ValidationConfig{Logger: appLogger})
	ctx := context.Background()

	now := time.Now().Unix()

	tests := []struct {
		name          string
		fill          *domain.Fill
		expectedError string
		isWarning     bool
	}{
		{
			name: "sent before received",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				ReceivedTimestamp:  float64(now - 3600),
				SentTimestamp:      float64(now - 3700), // Before received
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "sentTimestamp cannot be before receivedTimestamp",
			isWarning:     false,
		},
		{
			name: "last filled before sent",
			fill: &domain.Fill{
				ID:                  123,
				ExecutionServiceID:  456,
				ExecutionStatus:     "FULL",
				TradeType:           "BUY",
				Destination:         "ML",
				SecurityID:          "SEC123",
				Ticker:              "IBM",
				Quantity:            1000,
				ReceivedTimestamp:   float64(now - 3600),
				SentTimestamp:       float64(now - 3500),
				LastFilledTimestamp: float64(now - 3600), // Before sent
				QuantityFilled:      1000,
				AveragePrice:        190.41,
				Version:             1,
			},
			expectedError: "lastFilledTimestamp cannot be before sentTimestamp",
			isWarning:     false,
		},
		{
			name: "future timestamp warning",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				ReceivedTimestamp:  float64(now + 7200), // 2 hours in future
				SentTimestamp:      float64(now + 7300),
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "receivedTimestamp is in the future",
			isWarning:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ValidateFillMessage(ctx, tt.fill)

			if tt.isWarning {
				assert.True(t, result.IsValid)
				assert.NotEmpty(t, result.Warnings)

				found := false
				for _, warning := range result.Warnings {
					if warning.Message == tt.expectedError {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected warning message not found: %s", tt.expectedError)
			} else {
				assert.False(t, result.IsValid)
				assert.NotEmpty(t, result.Errors)

				found := false
				for _, err := range result.Errors {
					if err.Message == tt.expectedError {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error message not found: %s", tt.expectedError)
			}
		})
	}
}

func TestValidationService_ValidateFillMessage_FormatValidation(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewValidationService(ValidationConfig{Logger: appLogger})
	ctx := context.Background()

	tests := []struct {
		name          string
		fill          *domain.Fill
		expectedError string
	}{
		{
			name: "invalid ticker format",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "ibm123", // Should be uppercase
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "ticker 'ibm123' does not match expected format (1-5 uppercase letters)",
		},
		{
			name: "invalid destination format",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ml", // Should be uppercase
				SecurityID:         "SEC123",
				Ticker:             "IBM",
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "destination 'ml' does not match expected format (2-4 uppercase letters)",
		},
		{
			name: "ticker too long",
			fill: &domain.Fill{
				ID:                 123,
				ExecutionServiceID: 456,
				ExecutionStatus:    "FULL",
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				Ticker:             "VERYLONGTICKER", // Too long
				Quantity:           1000,
				QuantityFilled:     1000,
				AveragePrice:       190.41,
				Version:            1,
			},
			expectedError: "ticker exceeds maximum length of 10 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ValidateFillMessage(ctx, tt.fill)

			// These are warnings, not errors
			assert.True(t, result.IsValid || len(result.Errors) > 0)

			found := false
			for _, warning := range result.Warnings {
				if warning.Message == tt.expectedError {
					found = true
					break
				}
			}
			for _, err := range result.Errors {
				if err.Message == tt.expectedError {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected message not found: %s", tt.expectedError)
		})
	}
}

func TestValidationResult_GetErrorSummary(t *testing.T) {
	result := &ValidationResult{
		IsValid: false,
		Errors: []ValidationError{
			{Field: "field1", Code: "CODE1", Message: "Error 1"},
			{Field: "field2", Code: "CODE2", Message: "Error 2"},
		},
	}

	summary := result.GetErrorSummary()
	assert.Contains(t, summary, "field1: Error 1")
	assert.Contains(t, summary, "field2: Error 2")
}

func TestValidationResult_GetWarningSummary(t *testing.T) {
	result := &ValidationResult{
		IsValid: true,
		Warnings: []ValidationWarning{
			{Field: "field1", Code: "WARN1", Message: "Warning 1"},
			{Field: "field2", Code: "WARN2", Message: "Warning 2"},
		},
	}

	summary := result.GetWarningSummary()
	assert.Contains(t, summary, "field1: Warning 1")
	assert.Contains(t, summary, "field2: Warning 2")
}

func TestValidationResult_EmptySummaries(t *testing.T) {
	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	assert.Equal(t, "No validation errors", result.GetErrorSummary())
	assert.Equal(t, "No validation warnings", result.GetWarningSummary())
}
