package service

import (
	"context"
	"testing"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Helper function to create float64 pointer
func float64Ptr(f float64) *float64 {
	return &f
}

// MockExecutionServiceClient is a mock implementation of ExecutionServiceClientInterface
type MockExecutionServiceClient struct {
	mock.Mock
}

func (m *MockExecutionServiceClient) GetExecution(ctx context.Context, executionID int64) (*domain.ExecutionResponse, error) {
	args := m.Called(ctx, executionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ExecutionResponse), args.Error(1)
}

func (m *MockExecutionServiceClient) UpdateExecution(ctx context.Context, executionID int64, updateReq *domain.ExecutionUpdateRequest) (*domain.ExecutionUpdateResponse, error) {
	args := m.Called(ctx, executionID, updateReq)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ExecutionUpdateResponse), args.Error(1)
}

func (m *MockExecutionServiceClient) IsHealthy(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *MockExecutionServiceClient) GetStats() map[string]interface{} {
	args := m.Called()
	return args.Get(0).(map[string]interface{})
}

// MockAllocationServiceClient is a mock implementation of AllocationServiceClientInterface
type MockAllocationServiceClient struct {
	mock.Mock
}

func (m *MockAllocationServiceClient) PostExecution(ctx context.Context, dto *domain.AllocationServiceExecutionDTO) error {
	args := m.Called(ctx, dto)
	return args.Error(0)
}

func TestNewConfirmationService(t *testing.T) {
	mockClient := &MockExecutionServiceClient{}
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	appMetrics := metrics.New(metrics.Config{
		Enabled:   true,
		Namespace: "test",
	})

	resilienceManager := utils.NewResilienceManager(
		utils.GetDefaultResilienceConfig(),
		appLogger,
		appMetrics,
	)

	config := ConfirmationServiceConfig{
		ExecutionClient:   mockClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: resilienceManager,
	}

	service := NewConfirmationService(config)

	assert.NotNil(t, service)
	assert.Equal(t, mockClient, service.executionClient)
	assert.Equal(t, appLogger, service.logger)
	assert.Equal(t, appMetrics, service.metrics)
	assert.Equal(t, resilienceManager, service.resilienceManager)
}

func TestConfirmationService_HandleFillMessage_Success(t *testing.T) {
	// Setup
	mockClient := &MockExecutionServiceClient{}
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	appMetrics := metrics.New(metrics.Config{
		Enabled:   true,
		Namespace: "test",
	})

	resilienceManager := utils.NewResilienceManager(
		utils.GetDefaultResilienceConfig(),
		appLogger,
		appMetrics,
	)

	tracingProvider, err := utils.NewTracingProvider(utils.TracingConfig{Enabled: true, ServiceName: "test", Exporter: "stdout"})
	require.NoError(t, err)

	service := NewConfirmationService(ConfirmationServiceConfig{
		ExecutionClient:   mockClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: resilienceManager,
		TracingProvider:   tracingProvider,
	})

	// Test data
	ctx := context.Background()
	fill := &domain.Fill{
		ID:                  123,
		ExecutionServiceID:  456,
		IsOpen:              false,
		ExecutionStatus:     "FULL",
		TradeType:           "BUY",
		Destination:         "ML",
		SecurityID:          "SEC123",
		Ticker:              "IBM",
		Quantity:            1000,
		ReceivedTimestamp:   1748354367.509362,
		SentTimestamp:       1748354367.512467,
		LastFilledTimestamp: 1748354504.1602714,
		QuantityFilled:      1000,
		AveragePrice:        190.41,
		NumberOfFills:       3,
		TotalAmount:         190410.0,
		Version:             1,
	}

	currentExecution := &domain.ExecutionResponse{
		ID:              456,
		ExecutionStatus: "PARTIAL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC123",
		Quantity:        1000,
		QuantityFilled:  500,
		AveragePrice:    float64Ptr(190.0),
		Version:         2,
	}

	updateResponse := &domain.ExecutionUpdateResponse{
		ID:              456,
		ExecutionStatus: "FULL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC123",
		Quantity:        1000,
		QuantityFilled:  1000,
		AveragePrice:    float64Ptr(190.41),
		Version:         3,
	}

	// Setup expectations
	mockClient.On("GetExecution", mock.Anything, int64(456)).Return(currentExecution, nil)
	mockClient.On("UpdateExecution", mock.Anything, int64(456), mock.AnythingOfType("*domain.ExecutionUpdateRequest")).Return(updateResponse, nil)

	// Execute
	err = service.HandleFillMessage(ctx, fill)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestConfirmationService_HandleFillMessage_GetExecutionError(t *testing.T) {
	// Setup
	mockClient := &MockExecutionServiceClient{}
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	appMetrics := metrics.New(metrics.Config{
		Enabled:   true,
		Namespace: "test",
	})

	resilienceManager := utils.NewResilienceManager(
		utils.GetDefaultResilienceConfig(),
		appLogger,
		appMetrics,
	)

	tracingProvider, err := utils.NewTracingProvider(utils.TracingConfig{Enabled: true, ServiceName: "test", Exporter: "stdout"})
	require.NoError(t, err)

	service := NewConfirmationService(ConfirmationServiceConfig{
		ExecutionClient:   mockClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: resilienceManager,
		TracingProvider:   tracingProvider,
	})

	// Test data
	ctx := context.Background()
	fill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		ExecutionStatus:    "FULL",
		TradeType:          "BUY",
		Destination:        "ML",
		SecurityID:         "SEC123",
		Ticker:             "IBM",
		Quantity:           1000,
		QuantityFilled:     1000,
		AveragePrice:       190.41,
	}

	// Setup expectations
	expectedError := domain.NewNotFoundError("execution", "execution not found")
	mockClient.On("GetExecution", mock.Anything, int64(456)).Return(nil, expectedError)

	// Execute
	err = service.HandleFillMessage(ctx, fill)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get execution 456")
	mockClient.AssertExpectations(t)
}

func TestConfirmationService_HandleFillMessage_ValidationError(t *testing.T) {
	// Setup
	mockClient := &MockExecutionServiceClient{}
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	appMetrics := metrics.New(metrics.Config{
		Enabled:   true,
		Namespace: "test",
	})

	resilienceManager := utils.NewResilienceManager(
		utils.GetDefaultResilienceConfig(),
		appLogger,
		appMetrics,
	)

	tracingProvider, err := utils.NewTracingProvider(utils.TracingConfig{Enabled: true, ServiceName: "test", Exporter: "stdout"})
	require.NoError(t, err)

	service := NewConfirmationService(ConfirmationServiceConfig{
		ExecutionClient:   mockClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: resilienceManager,
		TracingProvider:   tracingProvider,
	})

	// Test data - mismatched trade types
	ctx := context.Background()
	fill := &domain.Fill{
		ID:                 123,
		ExecutionServiceID: 456,
		ExecutionStatus:    "FULL",
		TradeType:          "BUY",
		Destination:        "ML",
		SecurityID:         "SEC123",
		Ticker:             "IBM",
		Quantity:           1000,
		QuantityFilled:     1000,
		AveragePrice:       190.41,
	}

	currentExecution := &domain.ExecutionResponse{
		ID:              456,
		ExecutionStatus: "PARTIAL",
		TradeType:       "SELL", // Different trade type
		Destination:     "ML",
		SecurityID:      "SEC123",
		Quantity:        1000,
		QuantityFilled:  500,
		AveragePrice:    float64Ptr(190.0),
		Version:         2,
	}

	// Setup expectations
	mockClient.On("GetExecution", mock.Anything, int64(456)).Return(currentExecution, nil)

	// Execute
	err = service.HandleFillMessage(ctx, fill)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fill message validation failed")
	assert.Contains(t, err.Error(), "trade_type_mismatch")
	mockClient.AssertExpectations(t)
}

func TestConfirmationService_validateFillMessage(t *testing.T) {
	// Setup
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	service := &ConfirmationService{
		logger: appLogger,
	}

	ctx := context.Background()

	tests := []struct {
		name          string
		fill          *domain.Fill
		execution     *domain.ExecutionResponse
		expectedError string
	}{
		{
			name: "valid fill message",
			fill: &domain.Fill{
				ExecutionServiceID:  456,
				TradeType:           "BUY",
				Destination:         "ML",
				SecurityID:          "SEC123",
				QuantityFilled:      1000,
				AveragePrice:        190.41,
				ReceivedTimestamp:   1748354367.509362,
				SentTimestamp:       1748354367.512467,
				LastFilledTimestamp: 1748354504.1602714,
			},
			execution: &domain.ExecutionResponse{
				ID:          456,
				TradeType:   "BUY",
				Destination: "ML",
				SecurityID:  "SEC123",
				Quantity:    1000,
			},
			expectedError: "",
		},
		{
			name: "execution ID mismatch",
			fill: &domain.Fill{
				ExecutionServiceID: 456,
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
			},
			execution: &domain.ExecutionResponse{
				ID:          789, // Different ID
				TradeType:   "BUY",
				Destination: "ML",
				SecurityID:  "SEC123",
			},
			expectedError: "execution_id_mismatch",
		},
		{
			name: "trade type mismatch",
			fill: &domain.Fill{
				ExecutionServiceID: 456,
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
			},
			execution: &domain.ExecutionResponse{
				ID:          456,
				TradeType:   "SELL", // Different trade type
				Destination: "ML",
				SecurityID:  "SEC123",
			},
			expectedError: "trade_type_mismatch",
		},
		{
			name: "quantity filled exceeds total",
			fill: &domain.Fill{
				ExecutionServiceID: 456,
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				QuantityFilled:     1500, // Exceeds total
				AveragePrice:       190.41,
			},
			execution: &domain.ExecutionResponse{
				ID:          456,
				TradeType:   "BUY",
				Destination: "ML",
				SecurityID:  "SEC123",
				Quantity:    1000, // Total quantity
			},
			expectedError: "quantity_filled_exceeds_total",
		},
		{
			name: "invalid average price",
			fill: &domain.Fill{
				ExecutionServiceID: 456,
				TradeType:          "BUY",
				Destination:        "ML",
				SecurityID:         "SEC123",
				QuantityFilled:     1000,
				AveragePrice:       -10.0, // Invalid price
			},
			execution: &domain.ExecutionResponse{
				ID:          456,
				TradeType:   "BUY",
				Destination: "ML",
				SecurityID:  "SEC123",
				Quantity:    1000,
			},
			expectedError: "invalid_average_price",
		},
		{
			name: "invalid timestamp order",
			fill: &domain.Fill{
				ExecutionServiceID:  456,
				TradeType:           "BUY",
				Destination:         "ML",
				SecurityID:          "SEC123",
				QuantityFilled:      1000,
				AveragePrice:        190.41,
				ReceivedTimestamp:   1748354367.512467,
				SentTimestamp:       1748354367.509362, // Before received
				LastFilledTimestamp: 1748354504.1602714,
			},
			execution: &domain.ExecutionResponse{
				ID:          456,
				TradeType:   "BUY",
				Destination: "ML",
				SecurityID:  "SEC123",
				Quantity:    1000,
			},
			expectedError: "invalid_timestamp_order",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateFillMessage(ctx, tt.fill, tt.execution)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestConfirmationService_IsHealthy(t *testing.T) {
	mockClient := &MockExecutionServiceClient{}
	service := &ConfirmationService{
		executionClient: mockClient,
	}

	ctx := context.Background()

	tests := []struct {
		name           string
		clientHealthy  bool
		expectedResult bool
	}{
		{
			name:           "healthy",
			clientHealthy:  true,
			expectedResult: true,
		},
		{
			name:           "unhealthy",
			clientHealthy:  false,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.On("IsHealthy", ctx).Return(tt.clientHealthy).Once()

			result := service.IsHealthy(ctx)

			assert.Equal(t, tt.expectedResult, result)
		})
	}

	mockClient.AssertExpectations(t)
}

func TestConfirmationService_GetStats(t *testing.T) {
	mockClient := &MockExecutionServiceClient{}
	mockResilienceManager := &MockResilienceManager{}

	service := &ConfirmationService{
		executionClient:   mockClient,
		resilienceManager: mockResilienceManager,
	}

	expectedClientStats := map[string]interface{}{
		"base_url": "http://test:8084",
	}

	expectedCBStats := utils.CircuitBreakerStats{
		State: utils.StateClosed,
	}

	expectedDLQStats := utils.DeadLetterQueueStats{
		TotalMessages: 0,
	}

	mockClient.On("GetStats").Return(expectedClientStats)
	mockResilienceManager.On("GetCircuitBreakerStats").Return(expectedCBStats)
	mockResilienceManager.On("GetDeadLetterQueueStats").Return(expectedDLQStats)

	stats := service.GetStats()

	assert.Equal(t, "globeco-confirmation-service", stats["service_name"])
	assert.Equal(t, expectedClientStats, stats["execution_client"])
	assert.Equal(t, expectedCBStats, stats["circuit_breaker"])
	assert.Equal(t, expectedDLQStats, stats["dead_letter_queue"])

	mockClient.AssertExpectations(t)
	mockResilienceManager.AssertExpectations(t)
}

// MockResilienceManager is a mock implementation of ResilienceManagerInterface
type MockResilienceManager struct {
	mock.Mock
}

func (m *MockResilienceManager) GetCircuitBreakerStats() utils.CircuitBreakerStats {
	args := m.Called()
	return args.Get(0).(utils.CircuitBreakerStats)
}

func (m *MockResilienceManager) GetDeadLetterQueueStats() utils.DeadLetterQueueStats {
	args := m.Called()
	return args.Get(0).(utils.DeadLetterQueueStats)
}

func (m *MockResilienceManager) AddToDeadLetterQueue(ctx context.Context, originalMessage interface{}, failureReason string, errorHistory []error, attemptCount int, metadata map[string]interface{}) error {
	args := m.Called(ctx, originalMessage, failureReason, errorHistory, attemptCount, metadata)
	return args.Error(0)
}

// Test: Successful Allocation Service call
func TestConfirmationService_HandleFillMessage_AllocationSuccess(t *testing.T) {
	mockExecClient := &MockExecutionServiceClient{}
	mockAllocClient := &MockAllocationServiceClient{}
	mockResilience := &MockResilienceManager{}
	appLogger, _ := logger.New(logger.Config{Level: "info", Format: "json", Output: "stdout", ServiceName: "test"})
	appMetrics := metrics.New(metrics.Config{Enabled: true, Namespace: "test"})
	tracingProvider, _ := utils.NewTracingProvider(utils.TracingConfig{Enabled: true, ServiceName: "test", Exporter: "stdout"})

	service := NewConfirmationService(ConfirmationServiceConfig{
		ExecutionClient:   mockExecClient,
		AllocationClient:  mockAllocClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: mockResilience,
		TracingProvider:   tracingProvider,
	})

	ctx := context.Background()
	fill := &domain.Fill{
		ID:                  1,
		ExecutionServiceID:  2,
		IsOpen:              false,
		ExecutionStatus:     "FULL",
		TradeType:           "BUY",
		Destination:         "ML",
		SecurityID:          "SEC1",
		Ticker:              "IBM",
		Quantity:            100,
		ReceivedTimestamp:   1,
		SentTimestamp:       2,
		LastFilledTimestamp: 3,
		QuantityFilled:      100,
		AveragePrice:        10.0,
		NumberOfFills:       1,
		TotalAmount:         1000.0,
		Version:             1,
	}
	execResp := &domain.ExecutionResponse{
		ID:              2,
		ExecutionStatus: "PARTIAL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC1",
		Quantity:        100,
		QuantityFilled:  50,
		AveragePrice:    float64Ptr(9.0),
		Version:         1,
	}
	updateResp := &domain.ExecutionUpdateResponse{
		ID:              2,
		ExecutionStatus: "FULL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC1",
		Quantity:        100,
		QuantityFilled:  100,
		AveragePrice:    float64Ptr(10.0),
		Version:         2,
	}
	mockExecClient.On("GetExecution", mock.Anything, int64(2)).Return(execResp, nil)
	mockExecClient.On("UpdateExecution", mock.Anything, int64(2), mock.AnythingOfType("*domain.ExecutionUpdateRequest")).Return(updateResp, nil)
	mockAllocClient.On("PostExecution", mock.Anything, mock.AnythingOfType("*domain.AllocationServiceExecutionDTO")).Return(nil)

	err := service.HandleFillMessage(ctx, fill)
	assert.NoError(t, err)
	mockExecClient.AssertExpectations(t)
	mockAllocClient.AssertExpectations(t)
}

// Test: Allocation Service failure should add to DLQ
func TestConfirmationService_HandleFillMessage_AllocationFailure_DLQ(t *testing.T) {
	mockExecClient := &MockExecutionServiceClient{}
	mockAllocClient := &MockAllocationServiceClient{}
	mockResilience := &MockResilienceManager{}
	appLogger, _ := logger.New(logger.Config{Level: "info", Format: "json", Output: "stdout", ServiceName: "test"})
	appMetrics := metrics.New(metrics.Config{Enabled: true, Namespace: "test"})
	tracingProvider, _ := utils.NewTracingProvider(utils.TracingConfig{Enabled: true, ServiceName: "test", Exporter: "stdout"})

	service := NewConfirmationService(ConfirmationServiceConfig{
		ExecutionClient:   mockExecClient,
		AllocationClient:  mockAllocClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: mockResilience,
		TracingProvider:   tracingProvider,
	})

	ctx := context.Background()
	fill := &domain.Fill{
		ID:                  1,
		ExecutionServiceID:  2,
		IsOpen:              false,
		ExecutionStatus:     "FULL",
		TradeType:           "BUY",
		Destination:         "ML",
		SecurityID:          "SEC1",
		Ticker:              "IBM",
		Quantity:            100,
		ReceivedTimestamp:   1,
		SentTimestamp:       2,
		LastFilledTimestamp: 3,
		QuantityFilled:      100,
		AveragePrice:        10.0,
		NumberOfFills:       1,
		TotalAmount:         1000.0,
		Version:             1,
	}
	execResp := &domain.ExecutionResponse{
		ID:              2,
		ExecutionStatus: "PARTIAL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC1",
		Quantity:        100,
		QuantityFilled:  50,
		AveragePrice:    float64Ptr(9.0),
		Version:         1,
	}
	updateResp := &domain.ExecutionUpdateResponse{
		ID:              2,
		ExecutionStatus: "FULL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC1",
		Quantity:        100,
		QuantityFilled:  100,
		AveragePrice:    float64Ptr(10.0),
		Version:         2,
	}
	mockExecClient.On("GetExecution", mock.Anything, int64(2)).Return(execResp, nil)
	mockExecClient.On("UpdateExecution", mock.Anything, int64(2), mock.AnythingOfType("*domain.ExecutionUpdateRequest")).Return(updateResp, nil)
	mockAllocClient.On("PostExecution", mock.Anything, mock.AnythingOfType("*domain.AllocationServiceExecutionDTO")).Return(assert.AnError)
	mockResilience.On("AddToDeadLetterQueue", mock.Anything, mock.Anything, "allocation-service failure", mock.Anything, 1, mock.MatchedBy(func(meta map[string]interface{}) bool {
		return meta["service"] == "allocation-service"
	})).Return(nil)

	err := service.HandleFillMessage(ctx, fill)
	assert.NoError(t, err)
	mockExecClient.AssertExpectations(t)
	mockAllocClient.AssertExpectations(t)
	mockResilience.AssertExpectations(t)
}

// Test: Both Execution and Allocation Service failures (should add two DLQ records)
func TestConfirmationService_HandleFillMessage_BothFailures_DLQ(t *testing.T) {
	mockExecClient := &MockExecutionServiceClient{}
	mockAllocClient := &MockAllocationServiceClient{}
	mockResilience := &MockResilienceManager{}
	appLogger, _ := logger.New(logger.Config{Level: "info", Format: "json", Output: "stdout", ServiceName: "test"})
	appMetrics := metrics.New(metrics.Config{Enabled: true, Namespace: "test"})
	tracingProvider, _ := utils.NewTracingProvider(utils.TracingConfig{Enabled: true, ServiceName: "test", Exporter: "stdout"})

	service := NewConfirmationService(ConfirmationServiceConfig{
		ExecutionClient:   mockExecClient,
		AllocationClient:  mockAllocClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: mockResilience,
		TracingProvider:   tracingProvider,
	})

	ctx := context.Background()
	fill := &domain.Fill{
		ID:                  1,
		ExecutionServiceID:  2,
		IsOpen:              false,
		ExecutionStatus:     "FULL",
		TradeType:           "BUY",
		Destination:         "ML",
		SecurityID:          "SEC1",
		Ticker:              "IBM",
		Quantity:            100,
		ReceivedTimestamp:   1,
		SentTimestamp:       2,
		LastFilledTimestamp: 3,
		QuantityFilled:      100,
		AveragePrice:        10.0,
		NumberOfFills:       1,
		TotalAmount:         1000.0,
		Version:             1,
	}
	execErr := assert.AnError
	mockExecClient.On("GetExecution", mock.Anything, int64(2)).Return(nil, execErr)
	mockResilience.On("AddToDeadLetterQueue", mock.Anything, fill, "execution-service failure", mock.Anything, 1, mock.MatchedBy(func(meta map[string]interface{}) bool {
		return meta["service"] == "execution-service"
	})).Return(nil)
	mockAllocClient.On("PostExecution", mock.Anything, mock.AnythingOfType("*domain.AllocationServiceExecutionDTO")).Return(assert.AnError)
	mockResilience.On("AddToDeadLetterQueue", mock.Anything, mock.Anything, "allocation-service failure", mock.Anything, 1, mock.MatchedBy(func(meta map[string]interface{}) bool {
		return meta["service"] == "allocation-service"
	})).Return(nil)

	err := service.HandleFillMessage(ctx, fill)
	assert.Error(t, err)
	mockExecClient.AssertExpectations(t)
	mockAllocClient.AssertExpectations(t)
	mockResilience.AssertExpectations(t)
}

// Test: isOpen == true (should not call Allocation Service)
func TestConfirmationService_HandleFillMessage_IsOpenTrue_NoAllocationCall(t *testing.T) {
	mockExecClient := &MockExecutionServiceClient{}
	mockAllocClient := &MockAllocationServiceClient{}
	mockResilience := &MockResilienceManager{}
	appLogger, _ := logger.New(logger.Config{Level: "info", Format: "json", Output: "stdout", ServiceName: "test"})
	appMetrics := metrics.New(metrics.Config{Enabled: true, Namespace: "test"})
	tracingProvider, _ := utils.NewTracingProvider(utils.TracingConfig{Enabled: true, ServiceName: "test", Exporter: "stdout"})

	service := NewConfirmationService(ConfirmationServiceConfig{
		ExecutionClient:   mockExecClient,
		AllocationClient:  mockAllocClient,
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: mockResilience,
		TracingProvider:   tracingProvider,
	})

	ctx := context.Background()
	fill := &domain.Fill{
		ID:                  1,
		ExecutionServiceID:  2,
		IsOpen:              true, // Not completed
		ExecutionStatus:     "PARTIAL",
		TradeType:           "BUY",
		Destination:         "ML",
		SecurityID:          "SEC1",
		Ticker:              "IBM",
		Quantity:            100,
		ReceivedTimestamp:   1,
		SentTimestamp:       2,
		LastFilledTimestamp: 3,
		QuantityFilled:      50,
		AveragePrice:        9.0,
		NumberOfFills:       1,
		TotalAmount:         450.0,
		Version:             1,
	}
	execResp := &domain.ExecutionResponse{
		ID:              2,
		ExecutionStatus: "PARTIAL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC1",
		Quantity:        100,
		QuantityFilled:  50,
		AveragePrice:    float64Ptr(9.0),
		Version:         1,
	}
	updateResp := &domain.ExecutionUpdateResponse{
		ID:              2,
		ExecutionStatus: "PARTIAL",
		TradeType:       "BUY",
		Destination:     "ML",
		SecurityID:      "SEC1",
		Quantity:        100,
		QuantityFilled:  50,
		AveragePrice:    float64Ptr(9.0),
		Version:         2,
	}
	mockExecClient.On("GetExecution", mock.Anything, int64(2)).Return(execResp, nil)
	mockExecClient.On("UpdateExecution", mock.Anything, int64(2), mock.AnythingOfType("*domain.ExecutionUpdateRequest")).Return(updateResp, nil)
	// AllocationServiceClient should NOT be called

	err := service.HandleFillMessage(ctx, fill)
	assert.NoError(t, err)
	mockExecClient.AssertExpectations(t)
	mockAllocClient.AssertNotCalled(t, "PostExecution", mock.Anything, mock.Anything)
}
