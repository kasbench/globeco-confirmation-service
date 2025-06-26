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
	"go.uber.org/zap"
)

// AllocationServiceClient handles HTTP communication with the Allocation Service
// POST /api/v1/executions
// See documentation/supplemental-requirement-1.md

type AllocationServiceClient struct {
	config            config.AllocationServiceConfig
	httpClient        *http.Client
	logger            *logger.Logger
	metrics           *metrics.Metrics
	resilienceManager *utils.ResilienceManager
	tracingProvider   *utils.TracingProvider
}

type AllocationServiceClientConfig struct {
	AllocationService config.AllocationServiceConfig
	Logger            *logger.Logger
	Metrics           *metrics.Metrics
	ResilienceManager *utils.ResilienceManager
	TracingProvider   *utils.TracingProvider
}

func NewAllocationServiceClient(cfg AllocationServiceClientConfig) *AllocationServiceClient {
	httpClient := &http.Client{
		Timeout: cfg.AllocationService.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  false,
		},
	}
	return &AllocationServiceClient{
		config:            cfg.AllocationService,
		httpClient:        httpClient,
		logger:            cfg.Logger,
		metrics:           cfg.Metrics,
		resilienceManager: cfg.ResilienceManager,
		tracingProvider:   cfg.TracingProvider,
	}
}

// PostExecution posts a completed trade to the Allocation Service
func (asc *AllocationServiceClient) PostExecution(ctx context.Context, dto *domain.AllocationServiceExecutionDTO) error {
	url := fmt.Sprintf("%s/api/v1/executions", asc.config.BaseURL)
	correlationID := logger.GetCorrelationID(ctx)

	asc.logger.WithContext(ctx).Debug("Posting execution to Allocation Service",
		zap.String("url", url),
		zap.Int64("execution_service_id", dto.ExecutionServiceID),
	)

	return asc.resilienceManager.ExecuteAPICall(ctx, "POST", url, func(ctx context.Context) error {
		// Start tracing span
		var span interface{}
		if asc.tracingProvider != nil {
			ctx, span = asc.tracingProvider.StartHTTPClientSpan(ctx, "POST", url)
			defer func() {
				if s, ok := span.(interface{ End() }); ok {
					s.End()
				}
			}()
		}

		// Marshal request body
		requestBody, err := json.Marshal(dto)
		if err != nil {
			return domain.NewValidationError("invalid request", "failed to marshal allocation execution DTO").WithCorrelationID(correlationID)
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
		if err != nil {
			return domain.NewExternalError("allocation-service", "failed to create request", err, true).WithCorrelationID(correlationID)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Correlation-ID", correlationID)

		// Make the request
		resp, err := asc.httpClient.Do(req)
		if err != nil {
			return domain.NewExternalError("allocation-service", "request failed", err, true).WithCorrelationID(correlationID)
		}
		defer resp.Body.Close()

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return domain.NewExternalError("allocation-service", "failed to read response body", err, true).WithCorrelationID(correlationID)
		}

		// Check status code
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			asc.logger.WithContext(ctx).Error("Allocation Service returned error",
				zap.Int("status_code", resp.StatusCode),
				zap.String("body", string(body)),
			)
			return domain.NewExternalError("allocation-service", fmt.Sprintf("unexpected status code: %d", resp.StatusCode), nil, true).WithCorrelationID(correlationID)
		}

		asc.logger.WithContext(ctx).Info("Successfully posted execution to Allocation Service",
			zap.Int64("execution_service_id", dto.ExecutionServiceID),
			zap.Int("status_code", resp.StatusCode),
		)
		return nil
	})
}
