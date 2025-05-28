package domain

import (
	"fmt"
)

// ErrorType represents the type of error
type ErrorType string

const (
	// ErrorTypeValidation represents validation errors
	ErrorTypeValidation ErrorType = "VALIDATION"
	// ErrorTypeNotFound represents resource not found errors
	ErrorTypeNotFound ErrorType = "NOT_FOUND"
	// ErrorTypeConflict represents conflict errors (e.g., version mismatch)
	ErrorTypeConflict ErrorType = "CONFLICT"
	// ErrorTypeExternal represents external service errors
	ErrorTypeExternal ErrorType = "EXTERNAL"
	// ErrorTypeInternal represents internal service errors
	ErrorTypeInternal ErrorType = "INTERNAL"
	// ErrorTypeTimeout represents timeout errors
	ErrorTypeTimeout ErrorType = "TIMEOUT"
	// ErrorTypeCircuitBreaker represents circuit breaker errors
	ErrorTypeCircuitBreaker ErrorType = "CIRCUIT_BREAKER"
)

// DomainError represents a domain-specific error
type DomainError struct {
	Type          ErrorType `json:"type"`
	Code          string    `json:"code"`
	Message       string    `json:"message"`
	Details       string    `json:"details,omitempty"`
	Cause         error     `json:"-"`
	Retryable     bool      `json:"retryable"`
	CorrelationID string    `json:"correlationId,omitempty"`
}

// Error implements the error interface
func (e *DomainError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s - %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause
func (e *DomainError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns whether the error is retryable
func (e *DomainError) IsRetryable() bool {
	return e.Retryable
}

// NewValidationError creates a new validation error
func NewValidationError(message, details string) *DomainError {
	return &DomainError{
		Type:      ErrorTypeValidation,
		Code:      "VALIDATION_FAILED",
		Message:   message,
		Details:   details,
		Retryable: false,
	}
}

// NewNotFoundError creates a new not found error
func NewNotFoundError(resource, id string) *DomainError {
	return &DomainError{
		Type:      ErrorTypeNotFound,
		Code:      "RESOURCE_NOT_FOUND",
		Message:   fmt.Sprintf("%s not found", resource),
		Details:   fmt.Sprintf("ID: %s", id),
		Retryable: false,
	}
}

// NewConflictError creates a new conflict error
func NewConflictError(message, details string) *DomainError {
	return &DomainError{
		Type:      ErrorTypeConflict,
		Code:      "CONFLICT",
		Message:   message,
		Details:   details,
		Retryable: false,
	}
}

// NewExternalError creates a new external service error
func NewExternalError(service, message string, cause error, retryable bool) *DomainError {
	return &DomainError{
		Type:      ErrorTypeExternal,
		Code:      "EXTERNAL_SERVICE_ERROR",
		Message:   fmt.Sprintf("%s service error: %s", service, message),
		Cause:     cause,
		Retryable: retryable,
	}
}

// NewInternalError creates a new internal service error
func NewInternalError(message string, cause error) *DomainError {
	return &DomainError{
		Type:      ErrorTypeInternal,
		Code:      "INTERNAL_ERROR",
		Message:   message,
		Cause:     cause,
		Retryable: false,
	}
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(operation string, cause error) *DomainError {
	return &DomainError{
		Type:      ErrorTypeTimeout,
		Code:      "TIMEOUT",
		Message:   fmt.Sprintf("Operation timed out: %s", operation),
		Cause:     cause,
		Retryable: true,
	}
}

// NewCircuitBreakerError creates a new circuit breaker error
func NewCircuitBreakerError(service string) *DomainError {
	return &DomainError{
		Type:      ErrorTypeCircuitBreaker,
		Code:      "CIRCUIT_BREAKER_OPEN",
		Message:   fmt.Sprintf("Circuit breaker is open for %s service", service),
		Retryable: true,
	}
}

// WithCorrelationID adds a correlation ID to the error
func (e *DomainError) WithCorrelationID(correlationID string) *DomainError {
	e.CorrelationID = correlationID
	return e
}
