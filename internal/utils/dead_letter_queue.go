package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"go.uber.org/zap"
)

// DeadLetterMessage represents a message in the dead letter queue
type DeadLetterMessage struct {
	ID               string                 `json:"id"`
	CorrelationID    string                 `json:"correlation_id"`
	OriginalMessage  interface{}            `json:"original_message"`
	FailureReason    string                 `json:"failure_reason"`
	ErrorHistory     []string               `json:"error_history"`
	AttemptCount     int                    `json:"attempt_count"`
	FirstFailureTime time.Time              `json:"first_failure_time"`
	LastFailureTime  time.Time              `json:"last_failure_time"`
	Metadata         map[string]interface{} `json:"metadata"`
	Topic            string                 `json:"topic,omitempty"`
	Partition        int                    `json:"partition,omitempty"`
	Offset           int64                  `json:"offset,omitempty"`
}

// DeadLetterQueueConfig represents dead letter queue configuration
type DeadLetterQueueConfig struct {
	Enabled         bool          // Whether DLQ is enabled
	MaxSize         int           // Maximum number of messages to store
	RetentionPeriod time.Duration // How long to keep messages
	FlushInterval   time.Duration // How often to flush old messages
	PersistToDisk   bool          // Whether to persist messages to disk
	FilePath        string        // File path for disk persistence
}

// DeadLetterQueueStats represents DLQ statistics
type DeadLetterQueueStats struct {
	TotalMessages     int64     `json:"total_messages"`
	CurrentSize       int       `json:"current_size"`
	OldestMessageTime time.Time `json:"oldest_message_time"`
	NewestMessageTime time.Time `json:"newest_message_time"`
	LastFlushTime     time.Time `json:"last_flush_time"`
}

// DeadLetterQueue handles failed messages
type DeadLetterQueue struct {
	config   DeadLetterQueueConfig
	messages []DeadLetterMessage
	stats    DeadLetterQueueStats
	mutex    sync.RWMutex
	logger   *logger.Logger
	metrics  *metrics.Metrics
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(config DeadLetterQueueConfig, appLogger *logger.Logger, appMetrics *metrics.Metrics) *DeadLetterQueue {
	// Set defaults
	if config.MaxSize <= 0 {
		config.MaxSize = 1000
	}
	if config.RetentionPeriod <= 0 {
		config.RetentionPeriod = 24 * time.Hour
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 1 * time.Hour
	}

	dlq := &DeadLetterQueue{
		config:   config,
		messages: make([]DeadLetterMessage, 0, config.MaxSize),
		logger:   appLogger,
		metrics:  appMetrics,
		stopCh:   make(chan struct{}),
	}

	// Start background cleanup if enabled
	if config.Enabled {
		dlq.wg.Add(1)
		go dlq.cleanupWorker()
	}

	return dlq
}

// Add adds a message to the dead letter queue
func (dlq *DeadLetterQueue) Add(ctx context.Context, originalMessage interface{}, failureReason string, errorHistory []error, attemptCount int, metadata map[string]interface{}) error {
	if !dlq.config.Enabled {
		return nil
	}

	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	// Convert error history to strings
	errorStrings := make([]string, len(errorHistory))
	for i, err := range errorHistory {
		if err != nil {
			errorStrings[i] = err.Error()
		}
	}

	// Create dead letter message
	dlMessage := DeadLetterMessage{
		ID:               generateMessageID(),
		CorrelationID:    logger.GetCorrelationID(ctx),
		OriginalMessage:  originalMessage,
		FailureReason:    failureReason,
		ErrorHistory:     errorStrings,
		AttemptCount:     attemptCount,
		FirstFailureTime: time.Now(), // This could be enhanced to track actual first failure
		LastFailureTime:  time.Now(),
		Metadata:         metadata,
	}

	// Add Kafka-specific metadata if available
	if metadata != nil {
		if topic, ok := metadata["topic"].(string); ok {
			dlMessage.Topic = topic
		}
		if partition, ok := metadata["partition"].(int); ok {
			dlMessage.Partition = partition
		}
		if offset, ok := metadata["offset"].(int64); ok {
			dlMessage.Offset = offset
		}
	}

	// Check if we need to remove old messages
	if len(dlq.messages) >= dlq.config.MaxSize {
		// Remove oldest message
		dlq.messages = dlq.messages[1:]
	}

	// Add new message
	dlq.messages = append(dlq.messages, dlMessage)

	// Update statistics
	dlq.stats.TotalMessages++
	dlq.stats.CurrentSize = len(dlq.messages)
	dlq.stats.NewestMessageTime = time.Now()
	if dlq.stats.OldestMessageTime.IsZero() && len(dlq.messages) > 0 {
		dlq.stats.OldestMessageTime = dlq.messages[0].FirstFailureTime
	}

	// Log the dead letter message
	dlq.logger.WithContext(ctx).Error("Message added to dead letter queue",
		zap.String("message_id", dlMessage.ID),
		zap.String("failure_reason", failureReason),
		zap.Int("attempt_count", attemptCount),
		zap.Int("error_count", len(errorHistory)),
		zap.Int("dlq_size", len(dlq.messages)),
	)

	// Record metrics
	if dlq.metrics != nil {
		// We could add a specific DLQ metric here if needed
	}

	// Persist to disk if configured
	if dlq.config.PersistToDisk {
		if err := dlq.persistMessage(dlMessage); err != nil {
			dlq.logger.WithContext(ctx).Warn("Failed to persist dead letter message to disk",
				zap.String("message_id", dlMessage.ID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// GetMessages returns all messages in the dead letter queue
func (dlq *DeadLetterQueue) GetMessages() []DeadLetterMessage {
	dlq.mutex.RLock()
	defer dlq.mutex.RUnlock()

	// Return a copy to avoid race conditions
	messages := make([]DeadLetterMessage, len(dlq.messages))
	copy(messages, dlq.messages)
	return messages
}

// GetMessageByID returns a specific message by ID
func (dlq *DeadLetterQueue) GetMessageByID(id string) (*DeadLetterMessage, bool) {
	dlq.mutex.RLock()
	defer dlq.mutex.RUnlock()

	for _, msg := range dlq.messages {
		if msg.ID == id {
			return &msg, true
		}
	}
	return nil, false
}

// RemoveMessage removes a message from the dead letter queue
func (dlq *DeadLetterQueue) RemoveMessage(ctx context.Context, id string) bool {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	for i, msg := range dlq.messages {
		if msg.ID == id {
			// Remove message
			dlq.messages = append(dlq.messages[:i], dlq.messages[i+1:]...)
			dlq.stats.CurrentSize = len(dlq.messages)

			dlq.logger.WithContext(ctx).Info("Message removed from dead letter queue",
				zap.String("message_id", id),
				zap.Int("dlq_size", len(dlq.messages)),
			)
			return true
		}
	}
	return false
}

// Clear removes all messages from the dead letter queue
func (dlq *DeadLetterQueue) Clear(ctx context.Context) {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	messageCount := len(dlq.messages)
	dlq.messages = dlq.messages[:0]
	dlq.stats.CurrentSize = 0

	dlq.logger.WithContext(ctx).Info("Dead letter queue cleared",
		zap.Int("removed_messages", messageCount),
	)
}

// GetStats returns current statistics
func (dlq *DeadLetterQueue) GetStats() DeadLetterQueueStats {
	dlq.mutex.RLock()
	defer dlq.mutex.RUnlock()
	return dlq.stats
}

// cleanupWorker runs in the background to clean up old messages
func (dlq *DeadLetterQueue) cleanupWorker() {
	defer dlq.wg.Done()

	ticker := time.NewTicker(dlq.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dlq.stopCh:
			return
		case <-ticker.C:
			dlq.cleanup()
		}
	}
}

// cleanup removes old messages based on retention period
func (dlq *DeadLetterQueue) cleanup() {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	if len(dlq.messages) == 0 {
		return
	}

	cutoff := time.Now().Add(-dlq.config.RetentionPeriod)
	originalSize := len(dlq.messages)

	// Find first message that should be kept
	keepIndex := 0
	for i, msg := range dlq.messages {
		if msg.LastFailureTime.After(cutoff) {
			keepIndex = i
			break
		}
	}

	// Remove old messages
	if keepIndex > 0 {
		dlq.messages = dlq.messages[keepIndex:]
		dlq.stats.CurrentSize = len(dlq.messages)
		dlq.stats.LastFlushTime = time.Now()

		// Update oldest message time
		if len(dlq.messages) > 0 {
			dlq.stats.OldestMessageTime = dlq.messages[0].FirstFailureTime
		} else {
			dlq.stats.OldestMessageTime = time.Time{}
		}

		removedCount := originalSize - len(dlq.messages)
		if dlq.logger != nil {
			dlq.logger.Info("Dead letter queue cleanup completed",
				zap.Int("removed_messages", removedCount),
				zap.Int("remaining_messages", len(dlq.messages)),
				zap.Duration("retention_period", dlq.config.RetentionPeriod),
			)
		}
	}
}

// persistMessage persists a message to disk (placeholder implementation)
func (dlq *DeadLetterQueue) persistMessage(message DeadLetterMessage) error {
	if dlq.config.FilePath == "" {
		return fmt.Errorf("no file path configured for persistence")
	}

	// This is a simple implementation - in production you might want to use
	// a more robust approach like appending to a log file or using a database
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// For now, just log that we would persist (actual file I/O omitted for simplicity)
	_ = data
	return nil
}

// Stop stops the dead letter queue and cleanup worker
func (dlq *DeadLetterQueue) Stop(ctx context.Context) {
	if dlq.config.Enabled {
		close(dlq.stopCh)
		dlq.wg.Wait()

		dlq.logger.WithContext(ctx).Info("Dead letter queue stopped",
			zap.Int("final_message_count", len(dlq.messages)),
		)
	}
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("dlq-%d", time.Now().UnixNano())
}

// GetDefaultDeadLetterQueueConfig returns a default DLQ configuration
func GetDefaultDeadLetterQueueConfig() DeadLetterQueueConfig {
	return DeadLetterQueueConfig{
		Enabled:         true,
		MaxSize:         1000,
		RetentionPeriod: 24 * time.Hour,
		FlushInterval:   1 * time.Hour,
		PersistToDisk:   false,
		FilePath:        "",
	}
}
