package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/config"
	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	consumermetrics "github.com/kasbench/globeco-confirmation-service/internal/metrics"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// KafkaConsumerService handles Kafka message consumption
type KafkaConsumerService struct {
	config            config.KafkaConfig
	performanceConfig config.PerformanceConfig
	reader            *kafka.Reader
	logger            *logger.Logger
	metrics           *metrics.Metrics
	consumerMetrics   *consumermetrics.ConsumerMetrics
	resilienceManager *utils.ResilienceManager
	tracingProvider   *utils.TracingProvider

	// Message processing
	messageHandler MessageHandler

	// Parallel processing
	messageCh    chan kafka.Message
	commitCh     chan kafka.Message
	workerCount  int
	batchSize    int
	commitTicker *time.Ticker

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
	Performance       config.PerformanceConfig
	Logger            *logger.Logger
	Metrics           *metrics.Metrics
	ConsumerMetrics   *consumermetrics.ConsumerMetrics
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
		MaxBytes:    10e6,                   // 10MB
		MaxWait:     500 * time.Millisecond, // Reduced for better throughput
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

	// Calculate batch size for commits (10% of buffer or min 10)
	batchSize := config.Performance.MessageBufferSize / 10
	if batchSize < 10 {
		batchSize = 10
	}

	return &KafkaConsumerService{
		config:            config.Kafka,
		performanceConfig: config.Performance,
		reader:            reader,
		logger:            config.Logger,
		metrics:           config.Metrics,
		consumerMetrics:   config.ConsumerMetrics,
		resilienceManager: config.ResilienceManager,
		tracingProvider:   config.TracingProvider,
		messageHandler:    config.MessageHandler,
		messageCh:         make(chan kafka.Message, config.Performance.MessageBufferSize),
		commitCh:          make(chan kafka.Message, config.Performance.MessageBufferSize),
		workerCount:       config.Performance.WorkerPoolSize,
		batchSize:         batchSize,
		stopCh:            make(chan struct{}),
		doneCh:            make(chan struct{}),
	}
}

// Start starts the Kafka consumer
func (kcs *KafkaConsumerService) Start(ctx context.Context) error {
	kcs.mutex.Lock()
	defer kcs.mutex.Unlock()

	if kcs.isRunning {
		return fmt.Errorf("kafka consumer is already running")
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

	// Start commit ticker for batch commits
	kcs.commitTicker = time.NewTicker(2 * time.Second)

	// Start message fetcher
	kcs.wg.Add(1)
	go kcs.fetchLoop(ctx)

	// Start worker goroutines
	for i := 0; i < kcs.workerCount; i++ {
		kcs.wg.Add(1)
		go kcs.workerLoop(ctx, i)
	}

	// Start commit handler
	kcs.wg.Add(1)
	go kcs.commitLoop(ctx)

	kcs.logger.WithContext(ctx).Info("Kafka consumer started successfully",
		zap.Int("worker_count", kcs.workerCount),
		zap.Int("buffer_size", kcs.performanceConfig.MessageBufferSize),
		zap.Int("batch_size", kcs.batchSize),
	)
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

	// Stop commit ticker
	if kcs.commitTicker != nil {
		kcs.commitTicker.Stop()
	}

	// Wait for all goroutines to finish
	kcs.wg.Wait()

	// Close channels
	close(kcs.messageCh)
	close(kcs.commitCh)

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
		"worker_count":   kcs.workerCount,
		"batch_size":     kcs.batchSize,
	}

	// Add channel stats
	if kcs.messageCh != nil {
		stats["message_queue_length"] = len(kcs.messageCh)
		stats["message_queue_capacity"] = cap(kcs.messageCh)
	}

	if kcs.commitCh != nil {
		stats["commit_queue_length"] = len(kcs.commitCh)
		stats["commit_queue_capacity"] = cap(kcs.commitCh)
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

// fetchLoop fetches messages from Kafka and sends them to workers
func (kcs *KafkaConsumerService) fetchLoop(ctx context.Context) {
	defer kcs.wg.Done()

	correlationID := logger.GenerateCorrelationID()
	ctx = logger.WithCorrelationIDContext(ctx, correlationID)

	kcs.logger.WithContext(ctx).Info("Starting Kafka message fetch loop")

	for {
		select {
		case <-kcs.stopCh:
			kcs.logger.WithContext(ctx).Info("Kafka fetch loop stopping")
			return
		case <-ctx.Done():
			kcs.logger.WithContext(ctx).Info("Kafka fetch loop cancelled")
			return
		default:
			// Fetch message with timeout
			fetchCtx, cancel := context.WithTimeout(ctx, kcs.config.FetchTimeout)
			pollStart := time.Now()
			message, err := kcs.reader.FetchMessage(fetchCtx)
			cancel()
			pollDuration := time.Since(pollStart).Seconds()

			if err != nil {
				// Context cancellation or deadline exceeded: no metrics recorded
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					continue
				}
				// Other errors: record poll error metrics, then log and continue
				kcs.consumerMetrics.RecordPollError(ctx, pollDuration)
				kcs.logger.WithContext(ctx).Error("Error fetching message", zap.Error(err))
				// Brief pause before retrying
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Success: record poll success metrics
			kcs.consumerMetrics.RecordPollSuccess(ctx, pollDuration, message.Topic, message.Partition)

			// Send message to workers (non-blocking)
			select {
			case kcs.messageCh <- message:
				// Message sent successfully
			case <-kcs.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

// workerLoop processes messages from the message channel
func (kcs *KafkaConsumerService) workerLoop(ctx context.Context, workerID int) {
	defer kcs.wg.Done()

	correlationID := logger.GenerateCorrelationID()
	ctx = logger.WithCorrelationIDContext(ctx, correlationID)

	kcs.logger.WithContext(ctx).Info("Starting Kafka worker",
		zap.Int("worker_id", workerID),
	)

	for {
		select {
		case <-kcs.stopCh:
			kcs.logger.WithContext(ctx).Info("Kafka worker stopping",
				zap.Int("worker_id", workerID),
			)
			return
		case <-ctx.Done():
			kcs.logger.WithContext(ctx).Info("Kafka worker cancelled",
				zap.Int("worker_id", workerID),
			)
			return
		case message, ok := <-kcs.messageCh:
			if !ok {
				return // Channel closed
			}

			if err := kcs.handleMessage(ctx, message, workerID); err != nil {
				kcs.logger.WithContext(ctx).Error("Error processing message",
					zap.Int("worker_id", workerID),
					zap.Error(err),
				)
				// Don't commit failed messages
				continue
			}

			// Send message for commit (non-blocking)
			select {
			case kcs.commitCh <- message:
				// Message queued for commit
			case <-kcs.stopCh:
				return
			case <-ctx.Done():
				return
			default:
				// Commit channel full, log warning but continue
				kcs.logger.WithContext(ctx).Warn("Commit channel full, message may be reprocessed")
			}
		}
	}
}

// commitLoop handles batch commits of processed messages
func (kcs *KafkaConsumerService) commitLoop(ctx context.Context) {
	defer kcs.wg.Done()

	correlationID := logger.GenerateCorrelationID()
	ctx = logger.WithCorrelationIDContext(ctx, correlationID)

	kcs.logger.WithContext(ctx).Info("Starting Kafka commit loop",
		zap.Int("batch_size", kcs.batchSize),
	)

	var messagesToCommit []kafka.Message

	for {
		select {
		case <-kcs.stopCh:
			// Commit any remaining messages before stopping
			if len(messagesToCommit) > 0 {
				kcs.commitBatch(ctx, messagesToCommit)
			}
			kcs.logger.WithContext(ctx).Info("Kafka commit loop stopping")
			return
		case <-ctx.Done():
			kcs.logger.WithContext(ctx).Info("Kafka commit loop cancelled")
			return
		case <-kcs.commitTicker.C:
			// Periodic commit
			if len(messagesToCommit) > 0 {
				kcs.commitBatch(ctx, messagesToCommit)
				messagesToCommit = messagesToCommit[:0] // Reset slice
			}
		case message, ok := <-kcs.commitCh:
			if !ok {
				return // Channel closed
			}

			messagesToCommit = append(messagesToCommit, message)

			// Commit when batch is full
			if len(messagesToCommit) >= kcs.batchSize {
				kcs.commitBatch(ctx, messagesToCommit)
				messagesToCommit = messagesToCommit[:0] // Reset slice
			}
		}
	}
}

// commitBatch commits a batch of messages
func (kcs *KafkaConsumerService) commitBatch(ctx context.Context, messages []kafka.Message) {
	if len(messages) == 0 {
		return
	}

	startTime := time.Now()

	if err := kcs.reader.CommitMessages(ctx, messages...); err != nil {
		kcs.logger.WithContext(ctx).Error("Failed to commit message batch",
			zap.Int("batch_size", len(messages)),
			zap.Error(err),
		)
		return
	}

	commitTime := time.Since(startTime)
	kcs.logger.WithContext(ctx).Debug("Committed message batch",
		zap.Int("batch_size", len(messages)),
		zap.Duration("commit_time", commitTime),
	)
}

// processMessage is deprecated - replaced by parallel processing
// Keeping for backward compatibility but not used in new implementation

// computeLatencyPtr resolves the message creation time and calculates latency.
// Returns nil if no valid creation time is found or latency should be skipped.
func computeLatencyPtr(message kafka.Message, rawValue []byte, completionTime time.Time) *float64 {
	creationTime, ok := consumermetrics.ResolveMessageCreationTime(message, rawValue)
	if !ok {
		return nil
	}
	latency, ok := consumermetrics.CalculateLatency(creationTime, completionTime)
	if !ok {
		return nil
	}
	return &latency
}

// handleMessage handles a single Kafka message
func (kcs *KafkaConsumerService) handleMessage(ctx context.Context, message kafka.Message, workerID int) error {
	startTime := time.Now()
	processingStart := time.Now()
	rawValue := message.Value

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
		zap.Int("worker_id", workerID),
	)

	// Parse the fill message
	var fill domain.Fill
	if err := json.Unmarshal(message.Value, &fill); err != nil {
		kcs.metrics.RecordMessageFailed()

		// Record consumer metrics for unmarshal failure
		processingDuration := time.Since(processingStart).Seconds()
		if processingDuration < 0 {
			processingDuration = 0
		}
		completionTime := time.Now()
		latencyPtr := computeLatencyPtr(message, rawValue, completionTime)
		kcs.consumerMetrics.RecordProcessingFailure(ctx, processingDuration, latencyPtr, message.Topic, message.Partition)

		return fmt.Errorf("failed to unmarshal fill message: %w", err)
	}

	// Validate the fill message
	if err := fill.Validate(); err != nil {
		kcs.metrics.RecordMessageFailed()

		// Record consumer metrics for validation failure
		processingDuration := time.Since(processingStart).Seconds()
		if processingDuration < 0 {
			processingDuration = 0
		}
		completionTime := time.Now()
		latencyPtr := computeLatencyPtr(message, rawValue, completionTime)
		kcs.consumerMetrics.RecordProcessingFailure(ctx, processingDuration, latencyPtr, message.Topic, message.Partition)

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
			"worker_id": workerID,
		},
	)

	if err != nil {
		kcs.metrics.RecordMessageFailed()

		// Record consumer metrics for resilience-wrapped handler failure
		processingDuration := time.Since(processingStart).Seconds()
		if processingDuration < 0 {
			processingDuration = 0
		}
		completionTime := time.Now()
		latencyPtr := computeLatencyPtr(message, rawValue, completionTime)
		kcs.consumerMetrics.RecordProcessingFailure(ctx, processingDuration, latencyPtr, message.Topic, message.Partition)

		kcs.logger.WithContext(ctx).Error("Failed to handle fill message",
			zap.Int64("fill_id", fill.ID),
			zap.Int("worker_id", workerID),
			zap.Error(err),
		)

		// Don't commit the message if processing failed
		return err
	}

	// Update metrics and state
	processingTime := time.Since(startTime)
	kcs.metrics.RecordMessageProcessed()
	kcs.metrics.RecordMessageProcessingTime(processingTime)

	// Record consumer metrics for successful processing
	processingDuration := time.Since(processingStart).Seconds()
	if processingDuration < 0 {
		processingDuration = 0
	}
	completionTime := time.Now()
	latencyPtr := computeLatencyPtr(message, rawValue, completionTime)
	kcs.consumerMetrics.RecordProcessingSuccess(ctx, processingDuration, latencyPtr, message.Topic, message.Partition)

	kcs.mutex.Lock()
	kcs.messageCount++
	kcs.lastMessage = time.Now()
	kcs.mutex.Unlock()

	kcs.logger.WithContext(ctx).Debug("Successfully processed fill message",
		zap.Int64("fill_id", fill.ID),
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
		zap.Duration("processing_time", processingTime),
		zap.Int64("total_messages", kcs.messageCount),
		zap.Int("worker_id", workerID),
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
