package service

import (
	"context"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
)

// ExecutionServiceClientInterface defines the interface for the Execution Service client
type ExecutionServiceClientInterface interface {
	GetExecution(ctx context.Context, executionID int64) (*domain.ExecutionResponse, error)
	UpdateExecution(ctx context.Context, executionID int64, updateReq *domain.ExecutionUpdateRequest) (*domain.ExecutionUpdateResponse, error)
	IsHealthy(ctx context.Context) bool
	GetStats() map[string]interface{}
}

// ResilienceManagerInterface defines the interface for the resilience manager
type ResilienceManagerInterface interface {
	GetCircuitBreakerStats() utils.CircuitBreakerStats
	GetDeadLetterQueueStats() utils.DeadLetterQueueStats
	AddToDeadLetterQueue(ctx context.Context, originalMessage interface{}, failureReason string, errorHistory []error, attemptCount int, metadata map[string]interface{}) error
}

// KafkaConsumerInterface defines the interface for the Kafka consumer
type KafkaConsumerInterface interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsHealthy(ctx context.Context) bool
	GetStats() map[string]interface{}
}

// Ensure our concrete types implement the interfaces
var _ ExecutionServiceClientInterface = (*ExecutionServiceClient)(nil)
var _ ResilienceManagerInterface = (*utils.ResilienceManager)(nil)
