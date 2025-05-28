package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/service"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// ConfirmationServiceInterface defines what the handlers need from confirmation service
type ConfirmationServiceInterface interface {
	IsHealthy(ctx context.Context) bool
	GetStats() map[string]interface{}
}

// Handlers contains all HTTP handlers for the confirmation service
type Handlers struct {
	confirmationService ConfirmationServiceInterface
	kafkaConsumer       service.KafkaConsumerInterface
	logger              *logger.Logger
	metrics             *metrics.Metrics
	startTime           time.Time
}

// HandlerConfig represents the configuration for API handlers
type HandlerConfig struct {
	ConfirmationService ConfirmationServiceInterface
	KafkaConsumer       service.KafkaConsumerInterface
	Logger              *logger.Logger
	Metrics             *metrics.Metrics
}

// HealthResponse represents the response structure for health endpoints
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version"`
	Uptime    string                 `json:"uptime"`
	Checks    map[string]HealthCheck `json:"checks,omitempty"`
	Message   string                 `json:"message,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}

// HealthCheck represents an individual health check result
type HealthCheck struct {
	Status    string        `json:"status"`
	Message   string        `json:"message,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// StatsResponse represents the response structure for stats endpoint
type StatsResponse struct {
	Service     string                 `json:"service"`
	Timestamp   time.Time              `json:"timestamp"`
	Uptime      string                 `json:"uptime"`
	Version     string                 `json:"version"`
	Environment string                 `json:"environment"`
	Stats       map[string]interface{} `json:"stats"`
	RequestID   string                 `json:"requestId,omitempty"`
}

// ErrorResponse represents the standard error response structure
type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"requestId,omitempty"`
	Code      int       `json:"code"`
}

// NewHandlers creates a new handlers instance
func NewHandlers(config HandlerConfig) *Handlers {
	return &Handlers{
		confirmationService: config.ConfirmationService,
		kafkaConsumer:       config.KafkaConsumer,
		logger:              config.Logger,
		metrics:             config.Metrics,
		startTime:           time.Now(),
	}
}

// LivenessHandler implements the /health/live endpoint
// Returns 200 OK if the service is running (basic liveness check)
func (h *Handlers) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := logger.GetCorrelationID(ctx)

	h.logger.WithContext(ctx).Debug("Liveness check requested")

	response := HealthResponse{
		Status:    "UP",
		Timestamp: time.Now(),
		Service:   "confirmation-service",
		Version:   "1.0.0", // TODO: Get from build info
		Uptime:    time.Since(h.startTime).String(),
		Message:   "Service is alive and running",
		RequestID: correlationID,
	}

	// Record health check metrics
	if h.metrics != nil {
		h.metrics.RecordHealthCheck("liveness", true, time.Since(time.Now()))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.WithContext(ctx).Error("Failed to encode liveness response", zap.Error(err))
	}

	h.logger.WithContext(ctx).Debug("Liveness check completed successfully")
}

// ReadinessHandler implements the /health/ready endpoint
// Returns 200 OK if service can connect to dependencies (Kafka and Execution Service)
// Returns 503 Service Unavailable if dependencies are unreachable
func (h *Handlers) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := logger.GetCorrelationID(ctx)

	h.logger.WithContext(ctx).Debug("Readiness check requested")

	// Perform dependency checks with timeout
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	checks := make(map[string]HealthCheck)
	overallStatus := "UP"
	statusCode := http.StatusOK

	// Check Kafka connectivity
	kafkaStart := time.Now()
	kafkaHealthy := false
	kafkaMessage := "Kafka connection failed"

	if h.kafkaConsumer != nil {
		kafkaHealthy = h.kafkaConsumer.IsHealthy(checkCtx)
		if kafkaHealthy {
			kafkaMessage = "Kafka connection healthy"
		}
	} else {
		kafkaMessage = "Kafka consumer not initialized"
	}

	checks["kafka"] = HealthCheck{
		Status:    getStatusString(kafkaHealthy),
		Message:   kafkaMessage,
		Duration:  time.Since(kafkaStart),
		Timestamp: time.Now(),
	}

	// Check Execution Service connectivity
	executionStart := time.Now()
	executionHealthy := false
	executionMessage := "Execution Service connection failed"

	if h.confirmationService != nil {
		executionHealthy = h.confirmationService.IsHealthy(checkCtx)
		if executionHealthy {
			executionMessage = "Execution Service connection healthy"
		}
	} else {
		executionMessage = "Confirmation service not initialized"
	}

	checks["execution_service"] = HealthCheck{
		Status:    getStatusString(executionHealthy),
		Message:   executionMessage,
		Duration:  time.Since(executionStart),
		Timestamp: time.Now(),
	}

	// Determine overall status
	if !kafkaHealthy || !executionHealthy {
		overallStatus = "DOWN"
		statusCode = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Service:   "confirmation-service",
		Version:   "1.0.0", // TODO: Get from build info
		Uptime:    time.Since(h.startTime).String(),
		Checks:    checks,
		RequestID: correlationID,
	}

	if overallStatus == "UP" {
		response.Message = "Service is ready to accept traffic"
	} else {
		response.Message = "Service is not ready - dependency checks failed"
	}

	// Record health check metrics
	if h.metrics != nil {
		h.metrics.RecordHealthCheck("readiness", overallStatus == "UP", time.Since(time.Now()))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.WithContext(ctx).Error("Failed to encode readiness response", zap.Error(err))
	}

	h.logger.WithContext(ctx).Info("Readiness check completed",
		zap.String("overall_status", overallStatus),
		zap.Bool("kafka_healthy", kafkaHealthy),
		zap.Bool("execution_service_healthy", executionHealthy),
	)
}

// MetricsHandler serves Prometheus metrics at /metrics endpoint
func (h *Handlers) MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// StatsHandler implements the /stats endpoint for operational statistics
func (h *Handlers) StatsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := logger.GetCorrelationID(ctx)

	h.logger.WithContext(ctx).Debug("Stats requested")

	// Collect stats from various components
	stats := make(map[string]interface{})

	// Add confirmation service stats
	if h.confirmationService != nil {
		stats["confirmation_service"] = h.confirmationService.GetStats()
	}

	// Add Kafka consumer stats
	if h.kafkaConsumer != nil {
		stats["kafka_consumer"] = h.kafkaConsumer.GetStats()
	}

	// Add runtime stats
	stats["runtime"] = map[string]interface{}{
		"uptime":     time.Since(h.startTime).String(),
		"start_time": h.startTime,
	}

	response := StatsResponse{
		Service:     "confirmation-service",
		Timestamp:   time.Now(),
		Uptime:      time.Since(h.startTime).String(),
		Version:     "1.0.0", // TODO: Get from build info
		Environment: getEnvironment(),
		Stats:       stats,
		RequestID:   correlationID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.WithContext(ctx).Error("Failed to encode stats response", zap.Error(err))
		h.writeErrorResponse(w, r, http.StatusInternalServerError, "Failed to encode response", err)
		return
	}

	h.logger.WithContext(ctx).Debug("Stats request completed successfully")
}

// VersionHandler implements the /version endpoint
func (h *Handlers) VersionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := logger.GetCorrelationID(ctx)

	response := map[string]interface{}{
		"service":    "confirmation-service",
		"version":    "1.0.0",                // TODO: Get from build info
		"build_time": "2025-01-27T00:00:00Z", // TODO: Get from build info
		"git_commit": "unknown",              // TODO: Get from build info
		"go_version": "1.23.4",
		"timestamp":  time.Now(),
		"uptime":     time.Since(h.startTime).String(),
		"request_id": correlationID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.WithContext(ctx).Error("Failed to encode version response", zap.Error(err))
	}
}

// RootHandler implements the root / endpoint with basic service information
func (h *Handlers) RootHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := logger.GetCorrelationID(ctx)

	response := map[string]interface{}{
		"service":     "GlobeCo Confirmation Service",
		"description": "Microservice for processing fill messages from Kafka and updating the Execution Service",
		"version":     "1.0.0",
		"status":      "running",
		"timestamp":   time.Now(),
		"uptime":      time.Since(h.startTime).String(),
		"endpoints": map[string]string{
			"health_live":  "/health/live",
			"health_ready": "/health/ready",
			"metrics":      "/metrics",
			"stats":        "/stats",
			"version":      "/version",
		},
		"request_id": correlationID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.WithContext(ctx).Error("Failed to encode root response", zap.Error(err))
	}
}

// writeErrorResponse writes a standardized error response
func (h *Handlers) writeErrorResponse(w http.ResponseWriter, r *http.Request, statusCode int, message string, err error) {
	ctx := r.Context()
	correlationID := logger.GetCorrelationID(ctx)

	errorResponse := ErrorResponse{
		Error:     http.StatusText(statusCode),
		Message:   message,
		Timestamp: time.Now(),
		RequestID: correlationID,
		Code:      statusCode,
	}

	if err != nil {
		h.logger.WithContext(ctx).Error("API error occurred",
			zap.Int("status_code", statusCode),
			zap.String("message", message),
			zap.Error(err),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if encodeErr := json.NewEncoder(w).Encode(errorResponse); encodeErr != nil {
		h.logger.WithContext(ctx).Error("Failed to encode error response", zap.Error(encodeErr))
	}
}

// Helper functions

func getStatusString(healthy bool) string {
	if healthy {
		return "UP"
	}
	return "DOWN"
}

func getEnvironment() string {
	// TODO: Get from configuration
	return "development"
}
