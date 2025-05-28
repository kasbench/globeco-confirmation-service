package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/config"
	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// KafkaConsumerService handles Kafka message consumption
type KafkaConsumerService struct {
	config            config.KafkaConfig
	reader            *kafka.Reader
	logger            *logger.Logger
	metrics           *metrics.Metrics
	resilienceManager *utils.ResilienceManager
	tracingProvider   *utils.TracingProvider

	// Message processing
	messageHandler MessageHandler

	// Control channels
	stopCh chan struct{}
	doneCh chan struct{}
	wg     sync.WaitGroup

	// State tracking
	isRunning    bool
	mutex        sync.RWMutex
	lastMessage  time.Time
	messageCount int64
}

// MessageHandler defines the interface for handling processed messages
type MessageHandler interface {
	HandleFillMessage(ctx context.Context, fill *domain.Fill) error
}

// KafkaConsumerConfig represents Kafka consumer configuration
type KafkaConsumerConfig struct {
	Kafka             config.KafkaConfig
	Logger            *logger.Logger
	Metrics           *metrics.Metrics
	ResilienceManager *utils.ResilienceManager
	TracingProvider   *utils.TracingProvider
	MessageHandler    MessageHandler
}

// NewKafkaConsumerService creates a new Kafka consumer service
func NewKafkaConsumerService(config KafkaConsumerConfig) *KafkaConsumerService {
	// Create Kafka reader
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     config.Kafka.Brokers,
		Topic:       config.Kafka.Topic,
		GroupID:     config.Kafka.ConsumerGroup,
		MinBytes:    1,
		MaxBytes:    10e6, // 10MB
		MaxWait:     1 * time.Second,
		StartOffset: kafka.LastOffset,

		// Error handling
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			config.Logger.Error("Kafka reader error",
				zap.String("message", fmt.Sprintf(msg, args...)),
			)
		}),

		// Dialer configuration for timeouts
		Dialer: &kafka.Dialer{
			Timeout:   config.Kafka.ConnectionTimeout,
			DualStack: true,
		},
	})

	return &KafkaConsumerService{
		config:            config.Kafka,
		reader:            reader,
		logger:            config.Logger,
		metrics:           config.Metrics,
		resilienceManager: config.ResilienceManager,
		tracingProvider:   config.TracingProvider,
		messageHandler:    config.MessageHandler,
		stopCh:            make(chan struct{}),
		doneCh:            make(chan struct{}),
	}
}

// Start starts the Kafka consumer
func (kcs *KafkaConsumerService) Start(ctx context.Context) error {
	kcs.mutex.Lock()
	defer kcs.mutex.Unlock()

	if kcs.isRunning {
		return fmt.Errorf("Kafka consumer is already running")
	}

	correlationID := logger.GenerateCorrelationID()
	ctx = logger.WithCorrelationIDContext(ctx, correlationID)

	kcs.logger.WithContext(ctx).Info("Starting Kafka consumer",
		zap.Strings("brokers", kcs.config.Brokers),
		zap.String("topic", kcs.config.Topic),
		zap.String("consumer_group", kcs.config.ConsumerGroup),
	)

	// Test connection
	if err := kcs.testConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}

	kcs.isRunning = true
	kcs.wg.Add(1)
	go kcs.consumeLoop(ctx)

	kcs.logger.WithContext(ctx).Info("Kafka consumer started successfully")
	return nil
}

// Stop stops the Kafka consumer
func (kcs *KafkaConsumerService) Stop(ctx context.Context) error {
	kcs.mutex.Lock()
	defer kcs.mutex.Unlock()

	if !kcs.isRunning {
		return nil
	}

	kcs.logger.WithContext(ctx).Info("Stopping Kafka consumer")

	// Signal stop
	close(kcs.stopCh)

	// Wait for consumer loop to finish
	kcs.wg.Wait()

	// Close reader
	if err := kcs.reader.Close(); err != nil {
		kcs.logger.WithContext(ctx).Warn("Error closing Kafka reader", zap.Error(err))
	}

	kcs.isRunning = false
	close(kcs.doneCh)

	kcs.logger.WithContext(ctx).Info("Kafka consumer stopped",
		zap.Int64("total_messages_processed", kcs.messageCount),
	)

	return nil
}

// IsHealthy checks if the Kafka consumer is healthy
func (kcs *KafkaConsumerService) IsHealthy(ctx context.Context) bool {
	kcs.mutex.RLock()
	defer kcs.mutex.RUnlock()

	if !kcs.isRunning {
		return false
	}

	// Check if we've received messages recently (within last 5 minutes)
	// This is optional - in production you might want different health criteria
	if !kcs.lastMessage.IsZero() && time.Since(kcs.lastMessage) > 5*time.Minute {
		kcs.logger.WithContext(ctx).Warn("No messages received recently",
			zap.Duration("time_since_last_message", time.Since(kcs.lastMessage)),
		)
	}

	// Test connection
	return kcs.testConnection(ctx) == nil
}

// GetStats returns consumer statistics
func (kcs *KafkaConsumerService) GetStats() map[string]interface{} {
	kcs.mutex.RLock()
	defer kcs.mutex.RUnlock()

	stats := map[string]interface{}{
		"is_running":     kcs.isRunning,
		"message_count":  kcs.messageCount,
		"last_message":   kcs.lastMessage,
		"brokers":        kcs.config.Brokers,
		"topic":          kcs.config.Topic,
		"consumer_group": kcs.config.ConsumerGroup,
	}

	// Add reader stats if available
	if kcs.reader != nil {
		readerStats := kcs.reader.Stats()
		stats["reader_stats"] = map[string]interface{}{
			"messages":   readerStats.Messages,
			"bytes":      readerStats.Bytes,
			"rebalances": readerStats.Rebalances,
			"timeouts":   readerStats.Timeouts,
			"errors":     readerStats.Errors,
		}
	}

	return stats
}

// consumeLoop is the main message consumption loop
func (kcs *KafkaConsumerService) consumeLoop(ctx context.Context) {
	defer kcs.wg.Done()

	correlationID := logger.GenerateCorrelationID()
	ctx = logger.WithCorrelationIDContext(ctx, correlationID)

	kcs.logger.WithContext(ctx).Info("Starting Kafka message consumption loop")

	for {
		select {
		case <-kcs.stopCh:
			kcs.logger.WithContext(ctx).Info("Kafka consumer loop stopping")
			return
		case <-ctx.Done():
			kcs.logger.WithContext(ctx).Info("Kafka consumer loop cancelled")
			return
		default:
			if err := kcs.processMessage(ctx); err != nil {
				kcs.logger.WithContext(ctx).Error("Error processing message", zap.Error(err))
				// Continue processing other messages
			}
		}
	}
}

// processMessage processes a single Kafka message
func (kcs *KafkaConsumerService) processMessage(ctx context.Context) error {
	// Set timeout for message fetch
	fetchCtx, cancel := context.WithTimeout(ctx, kcs.config.FetchTimeout)
	defer cancel()

	// Read message with resilience
	return kcs.resilienceManager.ExecuteKafkaOperation(
		fetchCtx,
		"consume_message",
		kcs.config.Topic,
		-1, // Partition unknown at this point
		-1, // Offset unknown at this point
		func(ctx context.Context) error {
			message, err := kcs.reader.FetchMessage(ctx)
			if err != nil {
				if err == context.DeadlineExceeded {
					// Timeout is expected, not an error
					return nil
				}
				return fmt.Errorf("failed to fetch message: %w", err)
			}

			// Process the message
			return kcs.handleMessage(ctx, message)
		},
	)
}

// handleMessage handles a single Kafka message
func (kcs *KafkaConsumerService) handleMessage(ctx context.Context, message kafka.Message) error {
	startTime := time.Now()

	// Generate correlation ID for this message
	correlationID := logger.GenerateCorrelationID()
	ctx = logger.WithCorrelationIDContext(ctx, correlationID)

	// Start tracing span
	var span interface{}
	if kcs.tracingProvider != nil {
		ctx, span = kcs.tracingProvider.StartKafkaConsumerSpan(
			ctx,
			message.Topic,
			message.Partition,
			message.Offset,
		)
		defer func() {
			if s, ok := span.(interface{ End() }); ok {
				s.End()
			}
		}()
	}

	kcs.logger.WithContext(ctx).Debug("Processing Kafka message",
		zap.String("topic", message.Topic),
		zap.Int("partition", message.Partition),
		zap.Int64("offset", message.Offset),
		zap.Int("message_size", len(message.Value)),
	)

	// Parse the fill message
	var fill domain.Fill
	if err := json.Unmarshal(message.Value, &fill); err != nil {
		kcs.metrics.RecordMessageFailed()
		return fmt.Errorf("failed to unmarshal fill message: %w", err)
	}

	// Validate the fill message
	if err := fill.Validate(); err != nil {
		kcs.metrics.RecordMessageFailed()
		return fmt.Errorf("invalid fill message: %w", err)
	}

	// Handle the message with resilience
	err := kcs.resilienceManager.ExecuteWithResilience(
		ctx,
		"handle_fill_message",
		func(ctx context.Context) error {
			return kcs.messageHandler.HandleFillMessage(ctx, &fill)
		},
		map[string]interface{}{
			"topic":     message.Topic,
			"partition": message.Partition,
			"offset":    message.Offset,
			"fill_id":   fill.ID,
		},
	)

	if err != nil {
		kcs.metrics.RecordMessageFailed()
		kcs.logger.WithContext(ctx).Error("Failed to handle fill message",
			zap.Int64("fill_id", fill.ID),
			zap.Error(err),
		)

		// Don't commit the message if processing failed
		return err
	}

	// Commit the message
	if err := kcs.reader.CommitMessages(ctx, message); err != nil {
		kcs.logger.WithContext(ctx).Error("Failed to commit message",
			zap.Int("partition", message.Partition),
			zap.Int64("offset", message.Offset),
			zap.Error(err),
		)
		return fmt.Errorf("failed to commit message: %w", err)
	}

	// Update metrics and state
	processingTime := time.Since(startTime)
	kcs.metrics.RecordMessageProcessed()
	kcs.metrics.RecordMessageProcessingTime(processingTime)

	kcs.mutex.Lock()
	kcs.messageCount++
	kcs.lastMessage = time.Now()
	kcs.mutex.Unlock()

	kcs.logger.WithContext(ctx).Info("Successfully processed fill message",
		zap.Int64("fill_id", fill.ID),
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
		zap.Duration("processing_time", processingTime),
		zap.Int64("total_messages", kcs.messageCount),
	)

	return nil
}

// testConnection tests the Kafka connection
func (kcs *KafkaConsumerService) testConnection(ctx context.Context) error {
	// Create a test context with timeout
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try to fetch metadata to test connection
	conn, err := kafka.DialContext(testCtx, "tcp", kcs.config.Brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka broker: %w", err)
	}
	defer conn.Close()

	// Test if topic exists
	partitions, err := conn.ReadPartitions(kcs.config.Topic)
	if err != nil {
		return fmt.Errorf("failed to read topic partitions: %w", err)
	}

	if len(partitions) == 0 {
		return fmt.Errorf("topic %s has no partitions", kcs.config.Topic)
	}

	return nil
}
