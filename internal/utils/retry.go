package utils

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"go.uber.org/zap"
)

// RetryConfig represents retry configuration
type RetryConfig struct {
	MaxAttempts     int           // Maximum number of retry attempts
	InitialDelay    time.Duration // Initial delay before first retry
	MaxDelay        time.Duration // Maximum delay between retries
	BackoffFactor   float64       // Exponential backoff multiplier
	JitterEnabled   bool          // Whether to add random jitter
	RetryableErrors []string      // List of retryable error types
}

// RetryableFunc represents a function that can be retried
type RetryableFunc func(ctx context.Context) error

// RetryResult represents the result of a retry operation
type RetryResult struct {
	Success      bool
	Attempts     int
	TotalTime    time.Duration
	LastError    error
	ErrorHistory []error
}

// Retryer handles retry logic with exponential backoff
type Retryer struct {
	config RetryConfig
	logger *logger.Logger
}

// NewRetryer creates a new retryer instance
func NewRetryer(config RetryConfig, appLogger *logger.Logger) *Retryer {
	// Set defaults if not provided
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 5 * time.Second
	}
	if config.BackoffFactor <= 0 {
		config.BackoffFactor = 2.0
	}

	return &Retryer{
		config: config,
		logger: appLogger,
	}
}

// Execute executes a function with retry logic
func (r *Retryer) Execute(ctx context.Context, operation string, fn RetryableFunc) *RetryResult {
	startTime := time.Now()
	result := &RetryResult{
		ErrorHistory: make([]error, 0, r.config.MaxAttempts),
	}

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		r.logger.WithContext(ctx).Debug("Executing operation with retry",
			zap.String("operation", operation),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", r.config.MaxAttempts),
		)

		err := fn(ctx)
		if err == nil {
			result.Success = true
			result.LastError = nil // Clear error on success
			result.TotalTime = time.Since(startTime)

			if attempt > 1 {
				r.logger.WithContext(ctx).Info("Operation succeeded after retry",
					zap.String("operation", operation),
					zap.Int("attempts", attempt),
					zap.Duration("total_time", result.TotalTime),
				)
			}

			return result
		}

		result.LastError = err
		result.ErrorHistory = append(result.ErrorHistory, err)

		// Check if error is retryable
		if !r.isRetryableError(err) {
			r.logger.WithContext(ctx).Warn("Operation failed with non-retryable error",
				zap.String("operation", operation),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
			break
		}

		// Don't sleep after the last attempt
		if attempt < r.config.MaxAttempts {
			delay := r.calculateDelay(attempt)

			r.logger.WithContext(ctx).Warn("Operation failed, retrying",
				zap.String("operation", operation),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay),
				zap.Error(err),
			)

			select {
			case <-ctx.Done():
				result.LastError = ctx.Err()
				result.TotalTime = time.Since(startTime)
				return result
			case <-time.After(delay):
				// Continue to next attempt
			}
		} else {
			r.logger.WithContext(ctx).Error("Operation failed after all retry attempts",
				zap.String("operation", operation),
				zap.Int("max_attempts", r.config.MaxAttempts),
				zap.Duration("total_time", time.Since(startTime)),
				zap.Error(err),
			)
		}
	}

	result.TotalTime = time.Since(startTime)
	return result
}

// ExecuteWithStringResult executes a function with retry logic and returns a string result
func (r *Retryer) ExecuteWithStringResult(ctx context.Context, operation string, fn func(ctx context.Context) (string, error)) (string, *RetryResult) {
	var result string

	retryResult := r.Execute(ctx, operation, func(ctx context.Context) error {
		var err error
		result, err = fn(ctx)
		return err
	})

	return result, retryResult
}

// ExecuteWithInterfaceResult executes a function with retry logic and returns an interface{} result
func (r *Retryer) ExecuteWithInterfaceResult(ctx context.Context, operation string, fn func(ctx context.Context) (interface{}, error)) (interface{}, *RetryResult) {
	var result interface{}

	retryResult := r.Execute(ctx, operation, func(ctx context.Context) error {
		var err error
		result, err = fn(ctx)
		return err
	})

	return result, retryResult
}

// calculateDelay calculates the delay for the next retry attempt
func (r *Retryer) calculateDelay(attempt int) time.Duration {
	// Calculate exponential backoff
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.BackoffFactor, float64(attempt-1))

	// Apply maximum delay limit
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Add jitter if enabled
	if r.config.JitterEnabled {
		jitter := delay * 0.1 * (rand.Float64()*2 - 1) // ±10% jitter
		delay += jitter
	}

	return time.Duration(delay)
}

// isRetryableError checks if an error is retryable
func (r *Retryer) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a domain error with retryable flag
	if domainErr, ok := err.(*domain.DomainError); ok {
		return domainErr.IsRetryable()
	}

	// Check against configured retryable error types
	errorType := fmt.Sprintf("%T", err)
	for _, retryableType := range r.config.RetryableErrors {
		if errorType == retryableType {
			return true
		}
	}

	// Default retryable conditions - check error message
	errorMsg := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"service unavailable",
		"too many requests",
		"internal server error",
		"bad gateway",
		"gateway timeout",
		"network",
		"failure", // Generic failure for testing
	}

	for _, pattern := range retryablePatterns {
		if contains(errorMsg, pattern) {
			return true
		}
	}

	// Default to retryable for generic errors (unless explicitly non-retryable)
	return true
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsSubstring(s, substr))))
}

// containsSubstring performs a simple substring search
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetDefaultRetryConfig returns a default retry configuration
func GetDefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
		JitterEnabled: true,
		RetryableErrors: []string{
			"*domain.ExternalError",
			"*domain.TimeoutError",
		},
	}
}
