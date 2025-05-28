package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/api"
	"github.com/kasbench/globeco-confirmation-service/internal/config"
	"github.com/kasbench/globeco-confirmation-service/internal/service"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.LoadFromEnvironment()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize structured logger
	appLogger, err := logger.New(logger.Config{
		Level:       cfg.Logging.Level,
		Format:      cfg.Logging.Format,
		Output:      cfg.Logging.Output,
		ServiceName: cfg.Tracing.ServiceName,
	})
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Initialize metrics
	appMetrics := metrics.New(metrics.Config{
		Namespace: cfg.Metrics.Namespace,
		Enabled:   cfg.Metrics.Enabled,
	})

	// Initialize tracing
	tracingProvider, err := utils.NewTracingProvider(utils.TracingConfig{
		Enabled:        cfg.Tracing.Enabled,
		ServiceName:    cfg.Tracing.ServiceName,
		ServiceVersion: cfg.Tracing.ServiceVersion,
		Exporter:       cfg.Tracing.Exporter,
	})
	if err != nil {
		log.Fatalf("Failed to initialize tracing: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Generate correlation ID for startup
	correlationID := logger.GenerateCorrelationID()
	ctx = logger.WithCorrelationIDContext(ctx, correlationID)

	appLogger.WithContext(ctx).Info("Starting GlobeCo Confirmation Service",
		zap.String("service", cfg.Tracing.ServiceName),
		zap.String("version", cfg.Tracing.ServiceVersion),
		zap.String("environment", config.GetEnvironment()),
		zap.String("http_address", cfg.GetHTTPAddress()),
		zap.Strings("kafka_brokers", cfg.Kafka.Brokers),
		zap.String("kafka_topic", cfg.Kafka.Topic),
		zap.String("kafka_consumer_group", cfg.Kafka.ConsumerGroup),
		zap.String("execution_service_url", cfg.ExecutionService.BaseURL),
	)

	// Log configuration details in debug mode
	if cfg.Logging.Level == "debug" {
		appLogger.WithContext(ctx).Debug("Configuration loaded",
			zap.Any("http", cfg.HTTP),
			zap.Any("kafka", cfg.Kafka),
			zap.Any("execution_service", cfg.ExecutionService),
			zap.Any("performance", cfg.Performance),
			zap.Any("health", cfg.Health),
		)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		appLogger.WithContext(ctx).Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	}()

	// Initialize resilience manager
	resilienceManager := utils.NewResilienceManager(utils.ResilienceConfig{
		RetryConfig: utils.RetryConfig{
			InitialDelay:  cfg.ExecutionService.RetryBackoff,
			MaxDelay:      5 * time.Second,
			BackoffFactor: 2.0,
		},
		CircuitBreakerConfig: utils.CircuitBreakerConfig{
			FailureThreshold: cfg.ExecutionService.CircuitBreaker.FailureThreshold,
			Timeout:          cfg.ExecutionService.CircuitBreaker.Timeout,
		},
		DeadLetterQueueConfig: utils.DeadLetterQueueConfig{
			MaxSize: 1000,
		},
		TimeoutConfig: utils.TimeoutConfig{
			ExecutionServiceTimeout: cfg.ExecutionService.Timeout,
			KafkaConsumerTimeout:    cfg.Kafka.ConsumerTimeout,
			DefaultOperationTimeout: 5 * time.Second,
		},
	}, appLogger, appMetrics)

	// Initialize Execution Service client
	executionClient := service.NewExecutionServiceClient(service.ExecutionServiceClientConfig{
		ExecutionService:  cfg.ExecutionService,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: resilienceManager,
		TracingProvider:   tracingProvider,
	})

	// Initialize validation service
	validationService := service.NewValidationService(service.ValidationConfig{
		Logger: appLogger,
	})

	// Initialize duplicate detection service
	duplicateDetection := service.NewDuplicateDetectionService(service.DuplicateDetectionConfig{
		Logger:          appLogger,
		RetentionPeriod: 24 * time.Hour,
		MaxEntries:      10000,
	})

	// Initialize confirmation service (message handler)
	confirmationService := service.NewConfirmationService(service.ConfirmationServiceConfig{
		ExecutionClient:    executionClient,
		Logger:             appLogger,
		Metrics:            appMetrics,
		ResilienceManager:  resilienceManager,
		TracingProvider:    tracingProvider,
		ValidationService:  validationService,
		DuplicateDetection: duplicateDetection,
	})

	// Initialize Kafka consumer
	kafkaConsumer := service.NewKafkaConsumerService(service.KafkaConsumerConfig{
		Kafka:             cfg.Kafka,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: resilienceManager,
		TracingProvider:   tracingProvider,
		MessageHandler:    confirmationService,
	})

	// Initialize HTTP server for health checks and metrics
	httpHandler := api.NewHandlers(api.HandlerConfig{
		ConfirmationService: confirmationService,
		KafkaConsumer:       kafkaConsumer,
		Logger:              appLogger,
		Metrics:             appMetrics,
	})

	router := api.NewRouter(api.RouterConfig{
		Handlers: httpHandler,
		Logger:   appLogger,
		Metrics:  appMetrics,
	})
	httpServer := &http.Server{
		Addr:         cfg.GetHTTPAddress(),
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	// Start HTTP server
	go func() {
		appLogger.WithContext(ctx).Info("Starting HTTP server", zap.String("address", cfg.GetHTTPAddress()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.WithContext(ctx).Error("HTTP server failed", zap.Error(err))
			cancel()
		}
	}()

	// Start Kafka consumer
	if err := kafkaConsumer.Start(ctx); err != nil {
		appLogger.WithContext(ctx).Fatal("Failed to start Kafka consumer", zap.Error(err))
	}

	appLogger.WithContext(ctx).Info("Service started successfully",
		zap.Duration("startup_grace_period", cfg.Health.StartupGracePeriod),
	)

	// Wait for shutdown signal
	<-ctx.Done()

	appLogger.WithContext(ctx).Info("Shutting down service...")

	// Implement graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop Kafka consumer
	if err := kafkaConsumer.Stop(shutdownCtx); err != nil {
		appLogger.WithContext(shutdownCtx).Error("Error stopping Kafka consumer", zap.Error(err))
	}

	// Stop HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		appLogger.WithContext(shutdownCtx).Error("Error stopping HTTP server", zap.Error(err))
	}

	// Shutdown tracing provider
	if tracingProvider != nil {
		if err := tracingProvider.Shutdown(shutdownCtx); err != nil {
			appLogger.WithContext(shutdownCtx).Error("Error shutting down tracing provider", zap.Error(err))
		}
	}

	select {
	case <-shutdownCtx.Done():
		appLogger.WithContext(ctx).Error("Shutdown timeout exceeded")
	default:
		appLogger.WithContext(ctx).Info("Service shutdown completed")
	}
}
