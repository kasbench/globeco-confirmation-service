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

func TestNewDuplicateDetectionService(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	}

	service := NewDuplicateDetectionService(config)

	assert.NotNil(t, service)
	assert.Equal(t, appLogger, service.logger)
	assert.Equal(t, time.Hour, service.retentionPeriod)
	assert.Equal(t, 1000, service.maxEntries)
	assert.NotNil(t, service.processedMessages)
	assert.NotNil(t, service.stopCleanup)
	assert.NotNil(t, service.cleanupDone)

	// Clean up
	service.Stop()
}

func TestNewDuplicateDetectionService_DefaultValues(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	config := DuplicateDetectionConfig{
		Logger: appLogger,
		// No retention period or max entries specified
	}

	service := NewDuplicateDetectionService(config)

	assert.Equal(t, 24*time.Hour, service.retentionPeriod) // Default 24 hours
	assert.Equal(t, 10000, service.maxEntries)             // Default 10k entries

	// Clean up
	service.Stop()
}

func TestDuplicateDetectionService_CheckDuplicate_NewMessage(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	})
	defer service.Stop()

	ctx := context.Background()
	fill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		QuantityFilled:     1000,
		AveragePrice:       190.41,
		Version:            1,
	}

	result := service.CheckDuplicate(ctx, fill)

	assert.False(t, result.IsDuplicate)
	assert.Nil(t, result.PreviousMessage)
	assert.True(t, result.ShouldProcess)
	assert.Equal(t, "New message, not previously processed", result.Reason)
}

func TestDuplicateDetectionService_CheckDuplicate_ExactDuplicate(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	})
	defer service.Stop()

	ctx := context.Background()
	fill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		QuantityFilled:     1000,
		AveragePrice:       190.41,
		Version:            1,
	}

	// Record the message as processed successfully
	service.RecordProcessedMessage(ctx, fill, true, time.Millisecond*100, "")

	// Check for duplicate
	result := service.CheckDuplicate(ctx, fill)

	assert.True(t, result.IsDuplicate)
	assert.NotNil(t, result.PreviousMessage)
	assert.False(t, result.ShouldProcess) // Should not process exact duplicate
	assert.Equal(t, "Exact duplicate, skipping processing (idempotent operation)", result.Reason)
}

func TestDuplicateDetectionService_CheckDuplicate_FailedPrevious(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	})
	defer service.Stop()

	ctx := context.Background()
	fill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		QuantityFilled:     1000,
		AveragePrice:       190.41,
		Version:            1,
	}

	// Record the message as processed with failure
	service.RecordProcessedMessage(ctx, fill, false, time.Millisecond*100, "some error")

	// Check for duplicate
	result := service.CheckDuplicate(ctx, fill)

	assert.True(t, result.IsDuplicate)
	assert.NotNil(t, result.PreviousMessage)
	assert.True(t, result.ShouldProcess) // Should retry failed message
	assert.Equal(t, "Previous processing failed, retrying", result.Reason)
}

func TestDuplicateDetectionService_CheckDuplicate_SignificantChanges(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	})
	defer service.Stop()

	ctx := context.Background()
	originalFill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		QuantityFilled:     500,
		AveragePrice:       190.41,
		Version:            1,
	}

	// Record the original message as processed successfully
	service.RecordProcessedMessage(ctx, originalFill, true, time.Millisecond*100, "")

	// Create a message with significant changes
	updatedFill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		QuantityFilled:     1000, // Changed quantity
		AveragePrice:       190.41,
		Version:            1,
	}

	// Check for duplicate
	result := service.CheckDuplicate(ctx, updatedFill)

	assert.True(t, result.IsDuplicate)
	assert.NotNil(t, result.PreviousMessage)
	assert.True(t, result.ShouldProcess) // Should process due to significant changes
	assert.Equal(t, "Message has significant changes, processing as correction", result.Reason)
}

func TestDuplicateDetectionService_RecordProcessedMessage(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	})
	defer service.Stop()

	ctx := logger.WithCorrelationIDContext(context.Background(), "test-correlation-id")
	fill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		QuantityFilled:     1000,
		AveragePrice:       190.41,
		Version:            1,
	}

	processingTime := time.Millisecond * 150
	errorMessage := "test error"

	// Record successful processing
	service.RecordProcessedMessage(ctx, fill, true, processingTime, "")

	// Verify the message was recorded
	messageKey := service.generateMessageKey(fill)
	service.mutex.RLock()
	processedMessage, exists := service.processedMessages[messageKey]
	service.mutex.RUnlock()

	assert.True(t, exists)
	assert.NotNil(t, processedMessage)
	assert.Equal(t, fill.ID, processedMessage.FillID)
	assert.Equal(t, fill.ExecutionServiceID, processedMessage.ExecutionServiceID)
	assert.Equal(t, "test-correlation-id", processedMessage.CorrelationID)
	assert.Equal(t, processingTime, processedMessage.ProcessingTime)
	assert.True(t, processedMessage.Success)
	assert.Empty(t, processedMessage.ErrorMessage)
	assert.Equal(t, fill.Version, processedMessage.Version)
	assert.Equal(t, fill.QuantityFilled, processedMessage.QuantityFilled)
	assert.Equal(t, fill.AveragePrice, processedMessage.AveragePrice)

	// Record failed processing
	service.RecordProcessedMessage(ctx, fill, false, processingTime, errorMessage)

	// Verify the message was updated
	service.mutex.RLock()
	processedMessage, exists = service.processedMessages[messageKey]
	service.mutex.RUnlock()

	assert.True(t, exists)
	assert.False(t, processedMessage.Success)
	assert.Equal(t, errorMessage, processedMessage.ErrorMessage)
}

func TestDuplicateDetectionService_GetProcessedMessageStats(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	})
	defer service.Stop()

	ctx := context.Background()

	// Record some successful and failed messages
	for i := 0; i < 5; i++ {
		fill := &domain.Fill{
			ID:                 int64(i),
			ExecutionServiceID: 456,
			QuantityFilled:     1000,
			AveragePrice:       190.41,
			Version:            1,
		}
		success := i%2 == 0 // Alternate success/failure
		service.RecordProcessedMessage(ctx, fill, success, time.Millisecond*100, "")
	}

	stats := service.GetProcessedMessageStats()

	assert.Equal(t, 5, stats["total_messages"])
	assert.Equal(t, 3, stats["success_count"])   // 0, 2, 4
	assert.Equal(t, 2, stats["failure_count"])   // 1, 3
	assert.Equal(t, 60.0, stats["success_rate"]) // 3/5 * 100
	assert.Equal(t, time.Hour.String(), stats["retention_period"])
	assert.Equal(t, 1000, stats["max_entries"])
	assert.NotNil(t, stats["oldest_message"])
	assert.NotNil(t, stats["newest_message"])
	assert.NotNil(t, stats["time_span"])
}

func TestDuplicateDetectionService_GetProcessedMessageStats_Empty(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      1000,
	})
	defer service.Stop()

	stats := service.GetProcessedMessageStats()

	assert.Equal(t, 0, stats["total_messages"])
	assert.Equal(t, 0, stats["success_count"])
	assert.Equal(t, 0, stats["failure_count"])
	assert.Nil(t, stats["oldest_message"])
	assert.Nil(t, stats["newest_message"])
	assert.Nil(t, stats["time_span"])
}

func TestDuplicateDetectionService_generateMessageKey(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger: appLogger,
	})
	defer service.Stop()

	fill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
	}

	key := service.generateMessageKey(fill)
	expected := "fill_123_exec_456"

	assert.Equal(t, expected, key)
}

func TestDuplicateDetectionService_hasSignificantChanges(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger: appLogger,
	})
	defer service.Stop()

	previous := &ProcessedMessage{
		QuantityFilled: 1000,
		AveragePrice:   100.0,
		Version:        1,
	}

	tests := []struct {
		name     string
		current  *domain.Fill
		expected bool
	}{
		{
			name: "no changes",
			current: &domain.Fill{
				QuantityFilled: 1000,
				AveragePrice:   100.0,
				Version:        1,
			},
			expected: false,
		},
		{
			name: "quantity changed",
			current: &domain.Fill{
				QuantityFilled: 1500,
				AveragePrice:   100.0,
				Version:        1,
			},
			expected: true,
		},
		{
			name: "price changed significantly",
			current: &domain.Fill{
				QuantityFilled: 1000,
				AveragePrice:   101.0, // 1% change
				Version:        1,
			},
			expected: true,
		},
		{
			name: "price changed slightly",
			current: &domain.Fill{
				QuantityFilled: 1000,
				AveragePrice:   100.05, // 0.05% change
				Version:        1,
			},
			expected: false,
		},
		{
			name: "version changed",
			current: &domain.Fill{
				QuantityFilled: 1000,
				AveragePrice:   100.0,
				Version:        2,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.hasSignificantChanges(tt.current, previous)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDuplicateDetectionService_MaxEntriesCleanup(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	// Set a small max entries for testing
	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: time.Hour,
		MaxEntries:      5, // Small limit for testing
	})
	defer service.Stop()

	ctx := context.Background()

	// Add more messages than the limit
	for i := 0; i < 10; i++ {
		fill := &domain.Fill{
			ID:                 int64(i),
			ExecutionServiceID: 456,
			QuantityFilled:     1000,
			AveragePrice:       190.41,
			Version:            1,
		}
		service.RecordProcessedMessage(ctx, fill, true, time.Millisecond*100, "")

		// Add a small delay to ensure different timestamps
		time.Sleep(time.Millisecond)
	}

	// Should have triggered cleanup to stay under limit
	service.mutex.RLock()
	messageCount := len(service.processedMessages)
	service.mutex.RUnlock()

	// Should be around 90% of max entries (4-5 messages)
	assert.LessOrEqual(t, messageCount, 5)
	assert.GreaterOrEqual(t, messageCount, 4)
}

func TestDuplicateDetectionService_Stop(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := NewDuplicateDetectionService(DuplicateDetectionConfig{
		Logger: appLogger,
	})

	// Stop should complete without hanging
	done := make(chan bool)
	go func() {
		service.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Stop() did not complete within 1 second")
	}
}
