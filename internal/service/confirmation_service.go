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
	executionClient    ExecutionServiceClientInterface
	allocationClient   AllocationServiceClientInterface
	logger             *logger.Logger
	metrics            *metrics.Metrics
	resilienceManager  ResilienceManagerInterface
	tracingProvider    *utils.TracingProvider
	validationService  *ValidationService
	duplicateDetection *DuplicateDetectionService
}

// ConfirmationServiceConfig represents the configuration for the confirmation service
type ConfirmationServiceConfig struct {
	ExecutionClient    ExecutionServiceClientInterface
	AllocationClient   AllocationServiceClientInterface
	Logger             *logger.Logger
	Metrics            *metrics.Metrics
	ResilienceManager  ResilienceManagerInterface
	TracingProvider    *utils.TracingProvider
	ValidationService  *ValidationService
	DuplicateDetection *DuplicateDetectionService
}

// AllocationServiceClientInterface defines the interface for the Allocation Service client
// NEW: For dependency injection and testing
type AllocationServiceClientInterface interface {
	PostExecution(ctx context.Context, dto *domain.AllocationServiceExecutionDTO) error
}

// NewConfirmationService creates a new confirmation service
func NewConfirmationService(config ConfirmationServiceConfig) *ConfirmationService {
	return &ConfirmationService{
		executionClient:    config.ExecutionClient,
		allocationClient:   config.AllocationClient,
		logger:             config.Logger,
		metrics:            config.Metrics,
		resilienceManager:  config.ResilienceManager,
		tracingProvider:    config.TracingProvider,
		validationService:  config.ValidationService,
		duplicateDetection: config.DuplicateDetection,
	}
}

// HandleFillMessage implements the MessageHandler interface
// This method implements the core business logic:
// 1. Comprehensive input validation
// 2. Duplicate detection and idempotent processing
// 3. Get current execution version from Execution Service
// 4. Business rule validation
// 5. Update execution with fill data
func (cs *ConfirmationService) HandleFillMessage(ctx context.Context, fill *domain.Fill) error {
	startTime := time.Now()
	var processingError error

	cs.logger.WithContext(ctx).Info("Processing fill message", zap.Int64("fill_id", fill.ID))

	// Start tracing span
	ctx, span := cs.tracingProvider.StartSpan(ctx, "handle_fill_message")
	defer span.End()

	// Defer recording the processing result for duplicate detection
	defer func() {
		if cs.duplicateDetection != nil {
			cs.duplicateDetection.RecordProcessedMessage(ctx, fill, processingError == nil, time.Since(startTime), getErrorMessage(processingError))
		}
	}()

	// Comprehensive input validation
	if err := cs.validateInitialFillMessage(ctx, fill); err != nil {
		processingError = err
		cs.metrics.RecordMessageFailed()
		return processingError
	}

	// Duplicate detection
	if skip, reason := cs.checkForDuplicates(ctx, fill); skip {
		cs.logger.WithContext(ctx).Info("Skipping duplicate message processing", zap.Int64("fill_id", fill.ID), zap.String("reason", reason))
		cs.metrics.RecordMessageProcessed()
		return nil
	}

	// Handle Execution Service call
	updateResponse, execServiceFailed, execErr := cs.handleExecutionServiceCall(ctx, fill)
	if execServiceFailed {
		processingError = execErr
	}

	// Handle Allocation Service call for completed trades
	cs.handleAllocationServiceCall(ctx, fill)

	if !execServiceFailed {
		cs.logSuccess(ctx, fill, updateResponse, time.Since(startTime))
		cs.metrics.RecordMessageProcessed()
		cs.metrics.RecordMessageProcessingTime(time.Since(startTime))
	}

	return processingError
}

func (cs *ConfirmationService) validateInitialFillMessage(ctx context.Context, fill *domain.Fill) error {
	if cs.validationService != nil {
		validationResult := cs.validationService.ValidateFillMessage(ctx, fill)
		if !validationResult.IsValid {
			return domain.NewValidationError("comprehensive_validation_failed", validationResult.GetErrorSummary())
		}
		if len(validationResult.Warnings) > 0 {
			cs.logger.WithContext(ctx).Warn("Fill message validation passed with warnings",
				zap.Int64("fill_id", fill.ID),
				zap.String("warnings", validationResult.GetWarningSummary()),
			)
		}
	}
	return nil
}

func (cs *ConfirmationService) checkForDuplicates(ctx context.Context, fill *domain.Fill) (bool, string) {
	if cs.duplicateDetection != nil {
		duplicateResult := cs.duplicateDetection.CheckDuplicate(ctx, fill)
		if duplicateResult.IsDuplicate && !duplicateResult.ShouldProcess {
			return true, duplicateResult.Reason
		}
		if duplicateResult.IsDuplicate {
			cs.logger.WithContext(ctx).Info("Processing duplicate message with changes",
				zap.Int64("fill_id", fill.ID),
				zap.String("reason", duplicateResult.Reason),
			)
		}
	}
	return false, ""
}

func (cs *ConfirmationService) logSuccess(ctx context.Context, fill *domain.Fill, updateResponse *domain.ExecutionUpdateResponse, duration time.Duration) {
	cs.logger.WithContext(ctx).Info("Successfully processed fill message",
		zap.Int64("fill_id", fill.ID),
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
		zap.Int("new_version", updateResponse.Version),
		zap.Duration("processing_time", duration),
		zap.String("final_status", updateResponse.ExecutionStatus),
	)
}

func getErrorMessage(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
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

// handleExecutionServiceCall handles the interaction with the Execution Service
func (cs *ConfirmationService) handleExecutionServiceCall(ctx context.Context, fill *domain.Fill) (*domain.ExecutionUpdateResponse, bool, error) {
	// Get current execution from Execution Service to retrieve version
	execution, err := cs.executionClient.GetExecution(ctx, fill.ExecutionServiceID)
	if err != nil {
		processingError := fmt.Errorf("failed to get execution %d: %w", fill.ExecutionServiceID, err)
		cs.metrics.RecordMessageFailed()
		if cs.resilienceManager != nil {
			_ = cs.resilienceManager.AddToDeadLetterQueue(ctx, fill, "execution-service failure", []error{err}, 1, map[string]interface{}{"service": "execution-service"})
		}
		return nil, true, processingError
	}

	// Business rule validation against current execution
	if err := cs.validateFillMessage(ctx, fill, execution); err != nil {
		processingError := fmt.Errorf("fill message validation failed: %w", err)
		cs.metrics.RecordMessageFailed()
		if cs.resilienceManager != nil {
			_ = cs.resilienceManager.AddToDeadLetterQueue(ctx, fill, "execution-service failure", []error{err}, 1, map[string]interface{}{"service": "execution-service"})
		}
		return nil, true, processingError
	}

	// Create update request using the current version
	updateRequest := fill.ToUpdateRequest(execution.Version)

	// Update execution in Execution Service
	updateResponse, err := cs.executionClient.UpdateExecution(ctx, fill.ExecutionServiceID, updateRequest)
	if err != nil {
		processingError := fmt.Errorf("failed to update execution %d: %w", fill.ExecutionServiceID, err)
		cs.metrics.RecordMessageFailed()
		if cs.resilienceManager != nil {
			_ = cs.resilienceManager.AddToDeadLetterQueue(ctx, fill, "execution-service failure", []error{err}, 1, map[string]interface{}{"service": "execution-service"})
		}
		return nil, true, processingError
	}

	return updateResponse, false, nil
}

// handleAllocationServiceCall handles the interaction with the Allocation Service
func (cs *ConfirmationService) handleAllocationServiceCall(ctx context.Context, fill *domain.Fill) {
	// TEMPORARY: Log the fill object before checking isOpen
	cs.logger.WithContext(ctx).Info("AllocationServiceCall: fill object", zap.Any("fill", fill))
	if !fill.IsOpen && cs.allocationClient != nil {
		allocationDTO := domain.NewAllocationServiceExecutionDTO(fill)
		err := cs.allocationClient.PostExecution(ctx, allocationDTO)
		if err != nil {
			cs.logger.WithContext(ctx).Error("Failed to post to Allocation Service",
				zap.Int64("fill_id", fill.ID),
				zap.Error(err),
			)
			if cs.resilienceManager != nil {
				_ = cs.resilienceManager.AddToDeadLetterQueue(ctx, allocationDTO, "allocation-service failure", []error{err}, 1, map[string]interface{}{"service": "allocation-service"})
			}
		}
	}
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

	// Add duplicate detection stats
	if cs.duplicateDetection != nil {
		stats["duplicate_detection"] = cs.duplicateDetection.GetProcessedMessageStats()
	}

	return stats
}

// Add to ConfirmationService for debugging allocationClient wiring
func (cs *ConfirmationService) HasAllocationClient() bool {
	return cs.allocationClient != nil
}

func (cs *ConfirmationService) AllocationClientType() string {
	if cs.allocationClient == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", cs.allocationClient)
}
