package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"go.uber.org/zap"
)

// ConfirmationService implements the core business logic for processing fill messages
type ConfirmationService struct {
	executionClient   ExecutionServiceClientInterface
	logger            *logger.Logger
	metrics           *metrics.Metrics
	resilienceManager ResilienceManagerInterface
	tracingProvider   *utils.TracingProvider
}

// ConfirmationServiceConfig represents the configuration for the confirmation service
type ConfirmationServiceConfig struct {
	ExecutionClient   ExecutionServiceClientInterface
	Logger            *logger.Logger
	Metrics           *metrics.Metrics
	ResilienceManager ResilienceManagerInterface
	TracingProvider   *utils.TracingProvider
}

// NewConfirmationService creates a new confirmation service
func NewConfirmationService(config ConfirmationServiceConfig) *ConfirmationService {
	return &ConfirmationService{
		executionClient:   config.ExecutionClient,
		logger:            config.Logger,
		metrics:           config.Metrics,
		resilienceManager: config.ResilienceManager,
		tracingProvider:   config.TracingProvider,
	}
}

// HandleFillMessage implements the MessageHandler interface
// This method implements the core business logic:
// 1. Receive fill message from Kafka
// 2. Get current execution version from Execution Service
// 3. Update execution with fill data
func (cs *ConfirmationService) HandleFillMessage(ctx context.Context, fill *domain.Fill) error {
	startTime := time.Now()

	cs.logger.WithContext(ctx).Info("Processing fill message",
		zap.Int64("fill_id", fill.ID),
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
		zap.String("execution_status", fill.ExecutionStatus),
		zap.String("trade_type", fill.TradeType),
		zap.String("ticker", fill.Ticker),
		zap.Int64("quantity_filled", fill.QuantityFilled),
		zap.Float64("average_price", fill.AveragePrice),
	)

	// Start tracing span for the entire operation
	var span interface{}
	if cs.tracingProvider != nil {
		ctx, span = cs.tracingProvider.StartSpan(ctx, "handle_fill_message")
		defer func() {
			if s, ok := span.(interface{ End() }); ok {
				s.End()
			}
		}()
	}

	// Step 1: Get current execution from Execution Service to retrieve version
	cs.logger.WithContext(ctx).Debug("Getting current execution version",
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
	)

	execution, err := cs.executionClient.GetExecution(ctx, fill.ExecutionServiceID)
	if err != nil {
		cs.metrics.RecordMessageFailed()
		return fmt.Errorf("failed to get execution %d: %w", fill.ExecutionServiceID, err)
	}

	cs.logger.WithContext(ctx).Debug("Retrieved current execution",
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
		zap.Int("current_version", execution.Version),
		zap.String("current_status", execution.ExecutionStatus),
		zap.Int64("current_quantity_filled", execution.QuantityFilled),
	)

	// Step 2: Validate business rules
	if err := cs.validateFillMessage(ctx, fill, execution); err != nil {
		cs.metrics.RecordMessageFailed()
		return fmt.Errorf("fill message validation failed: %w", err)
	}

	// Step 3: Create update request using the current version
	updateRequest := fill.ToUpdateRequest(execution.Version)

	cs.logger.WithContext(ctx).Debug("Created update request",
		zap.Int64("quantity_filled", updateRequest.QuantityFilled),
		zap.Float64("average_price", updateRequest.AveragePrice),
		zap.Int("version", updateRequest.Version),
	)

	// Step 4: Update execution in Execution Service
	cs.logger.WithContext(ctx).Debug("Updating execution",
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
	)

	updateResponse, err := cs.executionClient.UpdateExecution(ctx, fill.ExecutionServiceID, updateRequest)
	if err != nil {
		cs.metrics.RecordMessageFailed()
		return fmt.Errorf("failed to update execution %d: %w", fill.ExecutionServiceID, err)
	}

	// Step 5: Log successful completion and record metrics
	processingTime := time.Since(startTime)

	cs.logger.WithContext(ctx).Info("Successfully processed fill message",
		zap.Int64("fill_id", fill.ID),
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
		zap.Int("old_version", execution.Version),
		zap.Int("new_version", updateResponse.Version),
		zap.Duration("processing_time", processingTime),
		zap.String("final_status", updateResponse.ExecutionStatus),
	)

	// Record success metrics
	cs.metrics.RecordMessageProcessed()
	cs.metrics.RecordMessageProcessingTime(processingTime)

	return nil
}

// validateFillMessage validates business rules for the fill message
func (cs *ConfirmationService) validateFillMessage(ctx context.Context, fill *domain.Fill, currentExecution *domain.ExecutionResponse) error {
	// Check if execution IDs match
	if fill.ExecutionServiceID != currentExecution.ID {
		return domain.NewValidationError("execution_id_mismatch",
			fmt.Sprintf("fill execution ID %d does not match current execution ID %d",
				fill.ExecutionServiceID, currentExecution.ID))
	}

	// Check if trade types match
	if fill.TradeType != currentExecution.TradeType {
		return domain.NewValidationError("trade_type_mismatch",
			fmt.Sprintf("fill trade type %s does not match execution trade type %s",
				fill.TradeType, currentExecution.TradeType))
	}

	// Check if destinations match
	if fill.Destination != currentExecution.Destination {
		return domain.NewValidationError("destination_mismatch",
			fmt.Sprintf("fill destination %s does not match execution destination %s",
				fill.Destination, currentExecution.Destination))
	}

	// Check if security IDs match
	if fill.SecurityID != currentExecution.SecurityID {
		return domain.NewValidationError("security_id_mismatch",
			fmt.Sprintf("fill security ID %s does not match execution security ID %s",
				fill.SecurityID, currentExecution.SecurityID))
	}

	// Check if quantity filled is reasonable
	if fill.QuantityFilled > currentExecution.Quantity {
		return domain.NewValidationError("quantity_filled_exceeds_total",
			fmt.Sprintf("fill quantity filled %d exceeds total execution quantity %d",
				fill.QuantityFilled, currentExecution.Quantity))
	}

	// Check if quantity filled is not decreasing (unless it's a correction)
	if fill.QuantityFilled < currentExecution.QuantityFilled {
		cs.logger.WithContext(ctx).Warn("Fill quantity is less than current quantity - possible correction",
			zap.Int64("fill_quantity_filled", fill.QuantityFilled),
			zap.Int64("current_quantity_filled", currentExecution.QuantityFilled),
		)
	}

	// Check if average price is reasonable (basic sanity check)
	if fill.AveragePrice <= 0 {
		return domain.NewValidationError("invalid_average_price",
			fmt.Sprintf("fill average price %f must be positive", fill.AveragePrice))
	}

	if fill.AveragePrice > 10000 {
		cs.logger.WithContext(ctx).Warn("Fill average price is very high",
			zap.Float64("average_price", fill.AveragePrice),
			zap.String("ticker", fill.Ticker),
		)
	}

	// Check timestamps for logical ordering (using Unix timestamps)
	if fill.ReceivedTimestamp > 0 && fill.SentTimestamp > 0 {
		if fill.SentTimestamp < fill.ReceivedTimestamp {
			return domain.NewValidationError("invalid_timestamp_order",
				"sent timestamp cannot be before received timestamp")
		}
	}

	if fill.LastFilledTimestamp > 0 && fill.SentTimestamp > 0 {
		if fill.LastFilledTimestamp < fill.SentTimestamp {
			return domain.NewValidationError("invalid_timestamp_order",
				"last filled timestamp cannot be before sent timestamp")
		}
	}

	return nil
}

// IsHealthy checks if the confirmation service is healthy
func (cs *ConfirmationService) IsHealthy(ctx context.Context) bool {
	// Check if execution service is healthy
	return cs.executionClient.IsHealthy(ctx)
}

// GetStats returns service statistics
func (cs *ConfirmationService) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"service_name": "confirmation-service",
	}

	// Add execution client stats
	if cs.executionClient != nil {
		stats["execution_client"] = cs.executionClient.GetStats()
	}

	// Add resilience manager stats
	if cs.resilienceManager != nil {
		stats["circuit_breaker"] = cs.resilienceManager.GetCircuitBreakerStats()
		stats["dead_letter_queue"] = cs.resilienceManager.GetDeadLetterQueueStats()
	}

	return stats
}
