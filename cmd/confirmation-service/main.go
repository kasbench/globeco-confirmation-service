package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/config"
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
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		appLogger.WithContext(ctx).Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	}()

	// TODO: Initialize Kafka consumer with cfg.Kafka
	// TODO: Initialize Execution Service client with cfg.ExecutionService
	// TODO: Initialize HTTP server for health checks with cfg.HTTP
	// TODO: Start message processing with cfg.Performance settings

	// Use the initialized components (to avoid unused variable warnings)
	_ = appMetrics
	_ = tracingProvider

	appLogger.WithContext(ctx).Info("Service started successfully",
		zap.Duration("startup_grace_period", cfg.Health.StartupGracePeriod),
	)

	// Wait for shutdown signal
	<-ctx.Done()

	appLogger.WithContext(ctx).Info("Shutting down service...")

	// TODO: Implement graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// TODO: Stop Kafka consumer
	// TODO: Stop HTTP server
	// TODO: Close database connections
	// TODO: Shutdown tracing provider

	select {
	case <-shutdownCtx.Done():
		appLogger.WithContext(ctx).Error("Shutdown timeout exceeded")
	default:
		appLogger.WithContext(ctx).Info("Service shutdown completed")
	}
}
