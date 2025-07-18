package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/config"
	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

// ExecutionServiceClient handles HTTP communication with the Execution Service
type ExecutionServiceClient struct {
	config            config.ExecutionServiceConfig
	httpClient        *http.Client
	logger            *logger.Logger
	metrics           *metrics.Metrics
	resilienceManager *utils.ResilienceManager
	tracingProvider   *utils.TracingProvider
}

// ExecutionServiceClientConfig represents the configuration for the Execution Service client
type ExecutionServiceClientConfig struct {
	ExecutionService  config.ExecutionServiceConfig
	Logger            *logger.Logger
	Metrics           *metrics.Metrics
	ResilienceManager *utils.ResilienceManager
	TracingProvider   *utils.TracingProvider
}

// NewExecutionServiceClient creates a new Execution Service client
func NewExecutionServiceClient(config ExecutionServiceClientConfig) *ExecutionServiceClient {
	// Create base transport
	baseTransport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
	}

	// Wrap transport with OpenTelemetry instrumentation
	instrumentedTransport := otelhttp.NewTransport(baseTransport)

	// Create HTTP client with timeout and instrumented transport
	httpClient := &http.Client{
		Timeout:   config.ExecutionService.Timeout,
		Transport: instrumentedTransport,
	}

	return &ExecutionServiceClient{
		config:            config.ExecutionService,
		httpClient:        httpClient,
		logger:            config.Logger,
		metrics:           config.Metrics,
		resilienceManager: config.ResilienceManager,
		tracingProvider:   config.TracingProvider,
	}
}

// GetExecution retrieves an execution by ID from the Execution Service
func (esc *ExecutionServiceClient) GetExecution(ctx context.Context, executionID int64) (*domain.ExecutionResponse, error) {
	url := fmt.Sprintf("%s/api/v1/execution/%d", esc.config.BaseURL, executionID)

	correlationID := logger.GetCorrelationID(ctx)
	esc.logger.WithContext(ctx).Debug("Getting execution from Execution Service",
		zap.Int64("execution_id", executionID),
		zap.String("url", url),
	)

	var response *domain.ExecutionResponse

	err := esc.resilienceManager.ExecuteAPICall(ctx, "GET", url, func(ctx context.Context) error {
		// Start tracing span
		var span interface{}
		if esc.tracingProvider != nil {
			ctx, span = esc.tracingProvider.StartHTTPClientSpan(ctx, "GET", url)
			defer func() {
				if s, ok := span.(interface{ End() }); ok {
					s.End()
				}
			}()
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return domain.NewExternalError("execution-service", "failed to create request", err, true).
				WithCorrelationID(correlationID)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Correlation-ID", correlationID)

		// Make the request
		resp, err := esc.httpClient.Do(req)
		if err != nil {
			return domain.NewExternalError("execution-service", "request failed", err, true).
				WithCorrelationID(correlationID)
		}
		defer resp.Body.Close()

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return domain.NewExternalError("execution-service", "failed to read response body", err, true).
				WithCorrelationID(correlationID)
		}

		// Check status code
		if resp.StatusCode != http.StatusOK {
			return esc.handleErrorResponse(resp.StatusCode, body, correlationID)
		}

		// Parse response
		var execResp domain.ExecutionResponse
		if err := json.Unmarshal(body, &execResp); err != nil {
			return domain.NewExternalError("execution-service", "failed to parse response", err, false).
				WithCorrelationID(correlationID)
		}

		response = &execResp
		return nil
	})

	if err != nil {
		esc.logger.WithContext(ctx).Error("Failed to get execution",
			zap.Int64("execution_id", executionID),
			zap.Error(err),
		)
		return nil, err
	}

	esc.logger.WithContext(ctx).Info("Successfully retrieved execution",
		zap.Int64("execution_id", executionID),
		zap.Int("version", response.Version),
	)

	return response, nil
}

// UpdateExecution updates an execution in the Execution Service
func (esc *ExecutionServiceClient) UpdateExecution(ctx context.Context, executionID int64, updateReq *domain.ExecutionUpdateRequest) (*domain.ExecutionUpdateResponse, error) {
	url := fmt.Sprintf("%s/api/v1/execution/%d", esc.config.BaseURL, executionID)

	correlationID := logger.GetCorrelationID(ctx)
	esc.logger.WithContext(ctx).Debug("Updating execution in Execution Service",
		zap.Int64("execution_id", executionID),
		zap.String("url", url),
		zap.Int64("quantity_filled", updateReq.QuantityFilled),
		zap.Float64("average_price", updateReq.AveragePrice),
		zap.Int("version", updateReq.Version),
	)

	var response *domain.ExecutionUpdateResponse

	err := esc.resilienceManager.ExecuteAPICall(ctx, "PUT", url, func(ctx context.Context) error {
		// Start tracing span
		var span interface{}
		if esc.tracingProvider != nil {
			ctx, span = esc.tracingProvider.StartHTTPClientSpan(ctx, "PUT", url)
			defer func() {
				if s, ok := span.(interface{ End() }); ok {
					s.End()
				}
			}()
		}

		// Marshal request body
		requestBody, err := json.Marshal(updateReq)
		if err != nil {
			return domain.NewValidationError("invalid request", "failed to marshal update request").
				WithCorrelationID(correlationID)
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(requestBody))
		if err != nil {
			return domain.NewExternalError("execution-service", "failed to create request", err, true).
				WithCorrelationID(correlationID)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Correlation-ID", correlationID)

		// Make the request
		resp, err := esc.httpClient.Do(req)
		if err != nil {
			return domain.NewExternalError("execution-service", "request failed", err, true).
				WithCorrelationID(correlationID)
		}
		defer resp.Body.Close()

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return domain.NewExternalError("execution-service", "failed to read response body", err, true).
				WithCorrelationID(correlationID)
		}

		// Check status code
		if resp.StatusCode != http.StatusOK {
			return esc.handleErrorResponse(resp.StatusCode, body, correlationID)
		}

		// Parse response
		var updateResp domain.ExecutionUpdateResponse
		if err := json.Unmarshal(body, &updateResp); err != nil {
			return domain.NewExternalError("execution-service", "failed to parse response", err, false).
				WithCorrelationID(correlationID)
		}

		response = &updateResp
		return nil
	})

	if err != nil {
		esc.logger.WithContext(ctx).Error("Failed to update execution",
			zap.Int64("execution_id", executionID),
			zap.Error(err),
		)
		return nil, err
	}

	esc.logger.WithContext(ctx).Info("Successfully updated execution",
		zap.Int64("execution_id", executionID),
		zap.Int64("quantity_filled", updateReq.QuantityFilled),
		zap.Float64("average_price", updateReq.AveragePrice),
		zap.Int("new_version", response.Version),
	)

	return response, nil
}

// IsHealthy checks if the Execution Service is healthy
func (esc *ExecutionServiceClient) IsHealthy(ctx context.Context) bool {
	// Create a health check context with shorter timeout
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Use the Spring Boot Actuator health endpoint
	url := fmt.Sprintf("%s/actuator/health/liveness", esc.config.BaseURL)

	req, err := http.NewRequestWithContext(healthCtx, "GET", url, nil)
	if err != nil {
		esc.logger.WithContext(ctx).Warn("Failed to create health check request", zap.Error(err))
		return false
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Correlation-ID", logger.GetCorrelationID(ctx))

	resp, err := esc.httpClient.Do(req)
	if err != nil {
		esc.logger.WithContext(ctx).Warn("Execution Service health check failed", zap.Error(err))
		return false
	}
	defer resp.Body.Close()

	// Consider 200-299 as healthy (even if empty list)
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300

	if !healthy {
		esc.logger.WithContext(ctx).Warn("Execution Service health check returned unhealthy status",
			zap.Int("status_code", resp.StatusCode),
		)
	} else {
		esc.logger.WithContext(ctx).Debug("Execution Service health check passed",
			zap.Int("status_code", resp.StatusCode),
		)
	}

	return healthy
}

// GetStats returns client statistics
func (esc *ExecutionServiceClient) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"base_url":      esc.config.BaseURL,
		"timeout":       esc.config.Timeout.String(),
		"max_retries":   esc.config.MaxRetries,
		"retry_backoff": esc.config.RetryBackoff.String(),
		"circuit_breaker": map[string]interface{}{
			"failure_threshold": esc.config.CircuitBreaker.FailureThreshold,
			"timeout":           esc.config.CircuitBreaker.Timeout.String(),
		},
	}
}

// handleErrorResponse handles HTTP error responses
func (esc *ExecutionServiceClient) handleErrorResponse(statusCode int, body []byte, correlationID string) error {
	switch statusCode {
	case http.StatusNotFound:
		return domain.NewNotFoundError("execution", "execution not found").
			WithCorrelationID(correlationID)
	case http.StatusBadRequest:
		return domain.NewValidationError("bad request", string(body)).
			WithCorrelationID(correlationID)
	case http.StatusConflict:
		return domain.NewConflictError("execution", "version conflict").
			WithCorrelationID(correlationID)
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.NewExternalError("execution-service", "authentication/authorization failed", nil, false).
			WithCorrelationID(correlationID)
	case http.StatusTooManyRequests:
		return domain.NewExternalError("execution-service", "rate limit exceeded", nil, true).
			WithCorrelationID(correlationID)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return domain.NewExternalError("execution-service", fmt.Sprintf("server error: %d", statusCode), nil, true).
			WithCorrelationID(correlationID)
	default:
		return domain.NewExternalError("execution-service", fmt.Sprintf("unexpected status code: %d", statusCode), nil, true).
			WithCorrelationID(correlationID)
	}
}
