package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"go.uber.org/zap"
)

// ResilienceConfig represents the configuration for the resilience manager
type ResilienceConfig struct {
	RetryConfig           RetryConfig
	CircuitBreakerConfig  CircuitBreakerConfig
	DeadLetterQueueConfig DeadLetterQueueConfig
	TimeoutConfig         TimeoutConfig
}

// TimeoutConfig represents timeout configuration
type TimeoutConfig struct {
	KafkaConsumerTimeout    time.Duration // Timeout for Kafka consumer operations
	ExecutionServiceTimeout time.Duration // Timeout for Execution Service API calls
	DefaultOperationTimeout time.Duration // Default timeout for other operations
}

// ResilienceManager provides comprehensive error handling and resilience
type ResilienceManager struct {
	retryer         *Retryer
	circuitBreaker  *CircuitBreaker
	deadLetterQueue *DeadLetterQueue
	timeoutConfig   TimeoutConfig
	logger          *logger.Logger
	metrics         *metrics.Metrics
}

// NewResilienceManager creates a new resilience manager
func NewResilienceManager(config ResilienceConfig, appLogger *logger.Logger, appMetrics *metrics.Metrics) *ResilienceManager {
	// Set timeout defaults
	if config.TimeoutConfig.KafkaConsumerTimeout <= 0 {
		config.TimeoutConfig.KafkaConsumerTimeout = 30 * time.Second
	}
	if config.TimeoutConfig.ExecutionServiceTimeout <= 0 {
		config.TimeoutConfig.ExecutionServiceTimeout = 10 * time.Second
	}
	if config.TimeoutConfig.DefaultOperationTimeout <= 0 {
		config.TimeoutConfig.DefaultOperationTimeout = 5 * time.Second
	}

	return &ResilienceManager{
		retryer:         NewRetryer(config.RetryConfig, appLogger),
		circuitBreaker:  NewCircuitBreaker(config.CircuitBreakerConfig, appLogger, appMetrics),
		deadLetterQueue: NewDeadLetterQueue(config.DeadLetterQueueConfig, appLogger, appMetrics),
		timeoutConfig:   config.TimeoutConfig,
		logger:          appLogger,
		metrics:         appMetrics,
	}
}

// ExecuteWithResilience executes an operation with full resilience (retry + circuit breaker + DLQ)
func (rm *ResilienceManager) ExecuteWithResilience(ctx context.Context, operation string, fn func(ctx context.Context) error, metadata map[string]interface{}) error {
	// Add timeout to context
	timeoutCtx, cancel := rm.createTimeoutContext(ctx, operation)
	defer cancel()

	// Execute with circuit breaker protection
	err := rm.circuitBreaker.Execute(timeoutCtx, func(ctx context.Context) error {
		// Execute with retry logic
		result := rm.retryer.Execute(ctx, operation, fn)
		return result.LastError
	})

	// If all retries failed, add to dead letter queue
	if err != nil {
		retryResult := rm.retryer.Execute(timeoutCtx, operation, fn)
		if !retryResult.Success {
			dlqErr := rm.deadLetterQueue.Add(
				ctx,
				metadata,
				fmt.Sprintf("Operation '%s' failed after %d attempts", operation, retryResult.Attempts),
				retryResult.ErrorHistory,
				retryResult.Attempts,
				metadata,
			)
			if dlqErr != nil {
				rm.logger.WithContext(ctx).Error("Failed to add message to dead letter queue",
					zap.String("operation", operation),
					zap.Error(dlqErr),
				)
			}
		}
	}

	return err
}

// ExecuteWithResilienceAndResult executes an operation with resilience and returns a result
func (rm *ResilienceManager) ExecuteWithResilienceAndResult(ctx context.Context, operation string, fn func(ctx context.Context) (interface{}, error), metadata map[string]interface{}) (interface{}, error) {
	var result interface{}

	err := rm.ExecuteWithResilience(ctx, operation, func(ctx context.Context) error {
		var execErr error
		result, execErr = fn(ctx)
		return execErr
	}, metadata)

	return result, err
}

// ExecuteAPICall executes an API call with appropriate resilience settings
func (rm *ResilienceManager) ExecuteAPICall(ctx context.Context, method, url string, fn func(ctx context.Context) error) error {
	metadata := map[string]interface{}{
		"type":   "api_call",
		"method": method,
		"url":    url,
	}

	operation := fmt.Sprintf("API %s %s", method, url)

	// Add API-specific timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, rm.timeoutConfig.ExecutionServiceTimeout)
	defer cancel()

	startTime := time.Now()

	err := rm.ExecuteWithResilience(timeoutCtx, operation, fn, metadata)

	// Record API call metrics
	duration := time.Since(startTime)
	statusCode := 0
	if err != nil {
		statusCode = rm.extractStatusCodeFromError(err)
	} else {
		statusCode = 200
	}

	if rm.metrics != nil {
		rm.metrics.RecordAPICall(method, url, fmt.Sprintf("%d", statusCode), duration)
	}

	// Log API call
	rm.logger.LogAPICall(ctx, method, url, statusCode, duration, err)

	return err
}

// ExecuteKafkaOperation executes a Kafka operation with appropriate resilience settings
func (rm *ResilienceManager) ExecuteKafkaOperation(ctx context.Context, operation string, topic string, partition int, offset int64, fn func(ctx context.Context) error) error {
	metadata := map[string]interface{}{
		"type":      "kafka_operation",
		"topic":     topic,
		"partition": partition,
		"offset":    offset,
	}

	// Add Kafka-specific timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, rm.timeoutConfig.KafkaConsumerTimeout)
	defer cancel()

	startTime := time.Now()

	err := rm.ExecuteWithResilience(timeoutCtx, operation, fn, metadata)

	// Record Kafka metrics
	duration := time.Since(startTime)
	if rm.metrics != nil {
		if err != nil {
			rm.metrics.RecordKafkaConnectionError()
		} else {
			rm.metrics.RecordKafkaMessage()
		}
	}

	// Log Kafka operation
	rm.logger.LogKafkaMessage(ctx, operation, topic, partition, offset, duration)

	return err
}

// createTimeoutContext creates a context with appropriate timeout for the operation
func (rm *ResilienceManager) createTimeoutContext(ctx context.Context, operation string) (context.Context, context.CancelFunc) {
	var timeout time.Duration

	switch {
	case contains(operation, "API"):
		timeout = rm.timeoutConfig.ExecutionServiceTimeout
	case contains(operation, "kafka") || contains(operation, "Kafka"):
		timeout = rm.timeoutConfig.KafkaConsumerTimeout
	default:
		timeout = rm.timeoutConfig.DefaultOperationTimeout
	}

	return context.WithTimeout(ctx, timeout)
}

// extractStatusCodeFromError attempts to extract HTTP status code from error
func (rm *ResilienceManager) extractStatusCodeFromError(err error) int {
	if err == nil {
		return 200
	}

	// Check if it's a domain error with status code
	if domainErr, ok := err.(*domain.DomainError); ok {
		switch domainErr.Type {
		case domain.ErrorTypeNotFound:
			return 404
		case domain.ErrorTypeValidation:
			return 400
		case domain.ErrorTypeConflict:
			return 409
		case domain.ErrorTypeTimeout:
			return 408
		case domain.ErrorTypeExternal:
			return 502
		case domain.ErrorTypeCircuitBreaker:
			return 503
		default:
			return 500
		}
	}

	// Default to 500 for unknown errors
	return 500
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (rm *ResilienceManager) GetCircuitBreakerStats() CircuitBreakerStats {
	return rm.circuitBreaker.GetStats()
}

// GetDeadLetterQueueStats returns dead letter queue statistics
func (rm *ResilienceManager) GetDeadLetterQueueStats() DeadLetterQueueStats {
	return rm.deadLetterQueue.GetStats()
}

// GetDeadLetterMessages returns all messages in the dead letter queue
func (rm *ResilienceManager) GetDeadLetterMessages() []DeadLetterMessage {
	return rm.deadLetterQueue.GetMessages()
}

// RemoveDeadLetterMessage removes a message from the dead letter queue
func (rm *ResilienceManager) RemoveDeadLetterMessage(ctx context.Context, messageID string) bool {
	return rm.deadLetterQueue.RemoveMessage(ctx, messageID)
}

// ClearDeadLetterQueue clears all messages from the dead letter queue
func (rm *ResilienceManager) ClearDeadLetterQueue(ctx context.Context) {
	rm.deadLetterQueue.Clear(ctx)
}

// ResetCircuitBreaker manually resets the circuit breaker
func (rm *ResilienceManager) ResetCircuitBreaker(ctx context.Context) {
	rm.circuitBreaker.Reset(ctx)
}

// Stop stops all background workers
func (rm *ResilienceManager) Stop(ctx context.Context) {
	rm.deadLetterQueue.Stop(ctx)

	rm.logger.WithContext(ctx).Info("Resilience manager stopped")
}

// GetDefaultResilienceConfig returns a default resilience configuration
func GetDefaultResilienceConfig() ResilienceConfig {
	return ResilienceConfig{
		RetryConfig:           GetDefaultRetryConfig(),
		CircuitBreakerConfig:  GetDefaultCircuitBreakerConfig("execution-service"),
		DeadLetterQueueConfig: GetDefaultDeadLetterQueueConfig(),
		TimeoutConfig: TimeoutConfig{
			KafkaConsumerTimeout:    30 * time.Second,
			ExecutionServiceTimeout: 10 * time.Second,
			DefaultOperationTimeout: 5 * time.Second,
		},
	}
}
