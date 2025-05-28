package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomainError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *DomainError
		expected string
	}{
		{
			name: "error with details",
			err: &DomainError{
				Code:    "TEST_ERROR",
				Message: "Test message",
				Details: "Additional details",
			},
			expected: "TEST_ERROR: Test message - Additional details",
		},
		{
			name: "error without details",
			err: &DomainError{
				Code:    "TEST_ERROR",
				Message: "Test message",
			},
			expected: "TEST_ERROR: Test message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestDomainError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &DomainError{
		Code:  "TEST_ERROR",
		Cause: cause,
	}

	assert.Equal(t, cause, err.Unwrap())
}

func TestDomainError_IsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		retryable bool
	}{
		{"retryable error", true},
		{"non-retryable error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &DomainError{Retryable: tt.retryable}
			assert.Equal(t, tt.retryable, err.IsRetryable())
		})
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("Invalid input", "Field 'name' is required")

	assert.Equal(t, ErrorTypeValidation, err.Type)
	assert.Equal(t, "VALIDATION_FAILED", err.Code)
	assert.Equal(t, "Invalid input", err.Message)
	assert.Equal(t, "Field 'name' is required", err.Details)
	assert.False(t, err.Retryable)
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("Execution", "123")

	assert.Equal(t, ErrorTypeNotFound, err.Type)
	assert.Equal(t, "RESOURCE_NOT_FOUND", err.Code)
	assert.Equal(t, "Execution not found", err.Message)
	assert.Equal(t, "ID: 123", err.Details)
	assert.False(t, err.Retryable)
}

func TestNewConflictError(t *testing.T) {
	err := NewConflictError("Version mismatch", "Expected version 1, got 2")

	assert.Equal(t, ErrorTypeConflict, err.Type)
	assert.Equal(t, "CONFLICT", err.Code)
	assert.Equal(t, "Version mismatch", err.Message)
	assert.Equal(t, "Expected version 1, got 2", err.Details)
	assert.False(t, err.Retryable)
}

func TestNewExternalError(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewExternalError("ExecutionService", "Failed to connect", cause, true)

	assert.Equal(t, ErrorTypeExternal, err.Type)
	assert.Equal(t, "EXTERNAL_SERVICE_ERROR", err.Code)
	assert.Equal(t, "ExecutionService service error: Failed to connect", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
}

func TestNewInternalError(t *testing.T) {
	cause := errors.New("database connection failed")
	err := NewInternalError("Internal processing failed", cause)

	assert.Equal(t, ErrorTypeInternal, err.Type)
	assert.Equal(t, "INTERNAL_ERROR", err.Code)
	assert.Equal(t, "Internal processing failed", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.False(t, err.Retryable)
}

func TestNewTimeoutError(t *testing.T) {
	cause := errors.New("context deadline exceeded")
	err := NewTimeoutError("API call", cause)

	assert.Equal(t, ErrorTypeTimeout, err.Type)
	assert.Equal(t, "TIMEOUT", err.Code)
	assert.Equal(t, "Operation timed out: API call", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
}

func TestNewCircuitBreakerError(t *testing.T) {
	err := NewCircuitBreakerError("ExecutionService")

	assert.Equal(t, ErrorTypeCircuitBreaker, err.Type)
	assert.Equal(t, "CIRCUIT_BREAKER_OPEN", err.Code)
	assert.Equal(t, "Circuit breaker is open for ExecutionService service", err.Message)
	assert.True(t, err.Retryable)
}

func TestDomainError_WithCorrelationID(t *testing.T) {
	err := NewValidationError("Test error", "Test details")
	correlationID := "test-correlation-123"

	result := err.WithCorrelationID(correlationID)

	assert.Equal(t, correlationID, result.CorrelationID)
	assert.Equal(t, err, result) // Should return the same instance
}

func TestErrorTypes(t *testing.T) {
	// Test that all error types are defined correctly
	assert.Equal(t, "VALIDATION", string(ErrorTypeValidation))
	assert.Equal(t, "NOT_FOUND", string(ErrorTypeNotFound))
	assert.Equal(t, "CONFLICT", string(ErrorTypeConflict))
	assert.Equal(t, "EXTERNAL", string(ErrorTypeExternal))
	assert.Equal(t, "INTERNAL", string(ErrorTypeInternal))
	assert.Equal(t, "TIMEOUT", string(ErrorTypeTimeout))
	assert.Equal(t, "CIRCUIT_BREAKER", string(ErrorTypeCircuitBreaker))
}
