package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"go.uber.org/zap"
)

// DuplicateDetectionService handles duplicate message detection and idempotent processing
type DuplicateDetectionService struct {
	logger            *logger.Logger
	processedMessages map[string]*ProcessedMessage
	mutex             sync.RWMutex
	retentionPeriod   time.Duration
	maxEntries        int

	// Background cleanup
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// ProcessedMessage represents a previously processed message
type ProcessedMessage struct {
	FillID             int64         `json:"fillId"`
	ExecutionServiceID int64         `json:"executionServiceId"`
	ProcessedAt        time.Time     `json:"processedAt"`
	CorrelationID      string        `json:"correlationId"`
	ProcessingTime     time.Duration `json:"processingTime"`
	Success            bool          `json:"success"`
	ErrorMessage       string        `json:"errorMessage,omitempty"`
	Version            int           `json:"version"`
	QuantityFilled     int64         `json:"quantityFilled"`
	AveragePrice       float64       `json:"averagePrice"`
}

// DuplicateDetectionConfig represents the configuration for duplicate detection
type DuplicateDetectionConfig struct {
	Logger          *logger.Logger
	RetentionPeriod time.Duration // How long to keep processed message records
	MaxEntries      int           // Maximum number of entries to keep in memory
}

// DuplicateResult represents the result of duplicate detection
type DuplicateResult struct {
	IsDuplicate     bool
	PreviousMessage *ProcessedMessage
	ShouldProcess   bool
	Reason          string
}

// NewDuplicateDetectionService creates a new duplicate detection service
func NewDuplicateDetectionService(config DuplicateDetectionConfig) *DuplicateDetectionService {
	if config.RetentionPeriod == 0 {
		config.RetentionPeriod = 24 * time.Hour // Default 24 hours
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = 10000 // Default 10k entries
	}

	service := &DuplicateDetectionService{
		logger:            config.Logger,
		processedMessages: make(map[string]*ProcessedMessage),
		retentionPeriod:   config.RetentionPeriod,
		maxEntries:        config.MaxEntries,
		stopCleanup:       make(chan struct{}),
		cleanupDone:       make(chan struct{}),
	}

	// Start background cleanup goroutine
	go service.cleanupLoop()

	return service
}

// CheckDuplicate checks if a fill message is a duplicate and determines if it should be processed
func (dds *DuplicateDetectionService) CheckDuplicate(ctx context.Context, fill *domain.Fill) *DuplicateResult {
	messageKey := dds.generateMessageKey(fill)

	dds.mutex.RLock()
	previousMessage, exists := dds.processedMessages[messageKey]
	dds.mutex.RUnlock()

	result := &DuplicateResult{
		IsDuplicate:     exists,
		PreviousMessage: previousMessage,
		ShouldProcess:   true, // Default to processing
	}

	if !exists {
		// Not a duplicate, should process
		result.Reason = "New message, not previously processed"
		dds.logger.WithContext(ctx).Debug("Message not found in duplicate detection cache",
			zap.Int64("fill_id", fill.ID),
			zap.String("message_key", messageKey),
		)
		return result
	}

	// Message is a duplicate, determine if we should still process it
	dds.logger.WithContext(ctx).Info("Duplicate message detected",
		zap.Int64("fill_id", fill.ID),
		zap.String("message_key", messageKey),
		zap.Time("previous_processed_at", previousMessage.ProcessedAt),
		zap.Bool("previous_success", previousMessage.Success),
	)

	// Decision logic for duplicate processing
	if !previousMessage.Success {
		// Previous processing failed, should retry
		result.ShouldProcess = true
		result.Reason = "Previous processing failed, retrying"
		dds.logger.WithContext(ctx).Info("Reprocessing failed duplicate message",
			zap.Int64("fill_id", fill.ID),
			zap.String("previous_error", previousMessage.ErrorMessage),
		)
	} else if dds.hasSignificantChanges(fill, previousMessage) {
		// Message has significant changes, should process as correction
		result.ShouldProcess = true
		result.Reason = "Message has significant changes, processing as correction"
		dds.logger.WithContext(ctx).Info("Processing duplicate with significant changes",
			zap.Int64("fill_id", fill.ID),
			zap.Int64("previous_quantity", previousMessage.QuantityFilled),
			zap.Int64("current_quantity", fill.QuantityFilled),
			zap.Float64("previous_price", previousMessage.AveragePrice),
			zap.Float64("current_price", fill.AveragePrice),
		)
	} else {
		// Exact duplicate, skip processing
		result.ShouldProcess = false
		result.Reason = "Exact duplicate, skipping processing (idempotent operation)"
		dds.logger.WithContext(ctx).Info("Skipping exact duplicate message",
			zap.Int64("fill_id", fill.ID),
			zap.Duration("time_since_processed", time.Since(previousMessage.ProcessedAt)),
		)
	}

	return result
}

// RecordProcessedMessage records a message as processed
func (dds *DuplicateDetectionService) RecordProcessedMessage(ctx context.Context, fill *domain.Fill, success bool, processingTime time.Duration, errorMessage string) {
	messageKey := dds.generateMessageKey(fill)
	correlationID := logger.GetCorrelationID(ctx)

	processedMessage := &ProcessedMessage{
		FillID:             fill.ID,
		ExecutionServiceID: fill.ExecutionServiceID,
		ProcessedAt:        time.Now(),
		CorrelationID:      correlationID,
		ProcessingTime:     processingTime,
		Success:            success,
		ErrorMessage:       errorMessage,
		Version:            fill.Version,
		QuantityFilled:     fill.QuantityFilled,
		AveragePrice:       fill.AveragePrice,
	}

	dds.mutex.Lock()
	defer dds.mutex.Unlock()

	// Check if we need to clean up to stay under max entries
	if len(dds.processedMessages) >= dds.maxEntries {
		dds.cleanupOldEntries()
	}

	dds.processedMessages[messageKey] = processedMessage

	dds.logger.WithContext(ctx).Debug("Recorded processed message",
		zap.Int64("fill_id", fill.ID),
		zap.String("message_key", messageKey),
		zap.Bool("success", success),
		zap.Duration("processing_time", processingTime),
		zap.Int("total_cached_messages", len(dds.processedMessages)),
	)
}

// GetProcessedMessageStats returns statistics about processed messages
func (dds *DuplicateDetectionService) GetProcessedMessageStats() map[string]interface{} {
	dds.mutex.RLock()
	defer dds.mutex.RUnlock()

	totalMessages := len(dds.processedMessages)
	successCount := 0
	failureCount := 0
	oldestMessage := time.Now()
	newestMessage := time.Time{}

	for _, msg := range dds.processedMessages {
		if msg.Success {
			successCount++
		} else {
			failureCount++
		}

		if msg.ProcessedAt.Before(oldestMessage) {
			oldestMessage = msg.ProcessedAt
		}
		if msg.ProcessedAt.After(newestMessage) {
			newestMessage = msg.ProcessedAt
		}
	}

	stats := map[string]interface{}{
		"total_messages":   totalMessages,
		"success_count":    successCount,
		"failure_count":    failureCount,
		"success_rate":     float64(successCount) / float64(totalMessages) * 100,
		"retention_period": dds.retentionPeriod.String(),
		"max_entries":      dds.maxEntries,
	}

	if totalMessages > 0 {
		stats["oldest_message"] = oldestMessage
		stats["newest_message"] = newestMessage
		stats["time_span"] = newestMessage.Sub(oldestMessage).String()
	}

	return stats
}

// Stop stops the duplicate detection service and cleanup goroutine
func (dds *DuplicateDetectionService) Stop() {
	close(dds.stopCleanup)
	<-dds.cleanupDone
}

// generateMessageKey generates a unique key for a fill message
func (dds *DuplicateDetectionService) generateMessageKey(fill *domain.Fill) string {
	// Use fill ID and execution service ID to create a unique key
	// This allows for the same fill ID to be processed for different executions
	return fmt.Sprintf("fill_%d_exec_%d", fill.ID, fill.ExecutionServiceID)
}

// hasSignificantChanges determines if a duplicate message has significant changes
func (dds *DuplicateDetectionService) hasSignificantChanges(current *domain.Fill, previous *ProcessedMessage) bool {
	// Check for significant changes in key fields

	// Quantity filled changed
	if current.QuantityFilled != previous.QuantityFilled {
		return true
	}

	// Average price changed significantly (more than 0.1%)
	priceDiff := current.AveragePrice - previous.AveragePrice
	if priceDiff < 0 {
		priceDiff = -priceDiff
	}
	priceChangePercent := (priceDiff / previous.AveragePrice) * 100
	if priceChangePercent > 0.1 {
		return true
	}

	// Version changed (indicates an update)
	if current.Version != previous.Version {
		return true
	}

	return false
}

// cleanupLoop runs in the background to clean up old entries
func (dds *DuplicateDetectionService) cleanupLoop() {
	defer close(dds.cleanupDone)

	ticker := time.NewTicker(time.Hour) // Clean up every hour
	defer ticker.Stop()

	for {
		select {
		case <-dds.stopCleanup:
			return
		case <-ticker.C:
			dds.performCleanup()
		}
	}
}

// performCleanup removes old entries based on retention period
func (dds *DuplicateDetectionService) performCleanup() {
	dds.mutex.Lock()
	defer dds.mutex.Unlock()

	cutoffTime := time.Now().Add(-dds.retentionPeriod)
	initialCount := len(dds.processedMessages)

	for key, message := range dds.processedMessages {
		if message.ProcessedAt.Before(cutoffTime) {
			delete(dds.processedMessages, key)
		}
	}

	finalCount := len(dds.processedMessages)
	removedCount := initialCount - finalCount

	if removedCount > 0 {
		dds.logger.Info("Cleaned up old processed messages",
			zap.Int("removed_count", removedCount),
			zap.Int("remaining_count", finalCount),
			zap.Duration("retention_period", dds.retentionPeriod),
		)
	}
}

// cleanupOldEntries removes the oldest entries to stay under max entries limit
func (dds *DuplicateDetectionService) cleanupOldEntries() {
	if len(dds.processedMessages) < dds.maxEntries {
		return
	}

	// Find the oldest entries to remove
	type keyTime struct {
		key  string
		time time.Time
	}

	var entries []keyTime
	for key, message := range dds.processedMessages {
		entries = append(entries, keyTime{key: key, time: message.ProcessedAt})
	}

	// Sort by time (oldest first)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].time.After(entries[j].time) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Remove oldest entries to get under the limit
	targetSize := dds.maxEntries * 9 / 10 // Remove 10% extra to avoid frequent cleanup
	removeCount := len(entries) - targetSize

	for i := 0; i < removeCount && i < len(entries); i++ {
		delete(dds.processedMessages, entries[i].key)
	}

	dds.logger.Info("Cleaned up old entries due to size limit",
		zap.Int("removed_count", removeCount),
		zap.Int("remaining_count", len(dds.processedMessages)),
		zap.Int("max_entries", dds.maxEntries),
	)
}
