package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/config"
	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestExecutionClient builds an ExecutionServiceClient wired to the given base URL
// with fast retries so conflict-retry behavior can be exercised quickly.
func newTestExecutionClient(t *testing.T, baseURL string) *ExecutionServiceClient {
	t.Helper()

	appLogger, err := logger.New(logger.Config{
		Level:       "error",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	appMetrics := metrics.New(metrics.Config{Enabled: true, Namespace: "test"})

	resilienceManager := utils.NewResilienceManager(utils.ResilienceConfig{
		RetryConfig: utils.RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  time.Millisecond,
			MaxDelay:      5 * time.Millisecond,
			BackoffFactor: 2.0,
		},
		CircuitBreakerConfig:  utils.GetDefaultCircuitBreakerConfig("execution-service"),
		DeadLetterQueueConfig: utils.GetDefaultDeadLetterQueueConfig(),
		TimeoutConfig: utils.TimeoutConfig{
			ExecutionServiceTimeout: 5 * time.Second,
			KafkaConsumerTimeout:    5 * time.Second,
			DefaultOperationTimeout: 5 * time.Second,
		},
	}, appLogger, appMetrics)

	return NewExecutionServiceClient(ExecutionServiceClientConfig{
		ExecutionService: config.ExecutionServiceConfig{
			BaseURL:      baseURL,
			Timeout:      5 * time.Second,
			MaxRetries:   3,
			RetryBackoff: time.Millisecond,
		},
		Logger:            appLogger,
		Metrics:           appMetrics,
		ResilienceManager: resilienceManager,
	})
}

// TestUpdateExecutionWithRefetch_ReFetchesVersionBetweenAttempts verifies that an
// optimistic-lock conflict (surfaced as HTTP 500) causes the next retry attempt to
// re-GET a fresh version before issuing the PUT, allowing the update to self-heal.
func TestUpdateExecutionWithRefetch_ReFetchesVersionBetweenAttempts(t *testing.T) {
	var mu sync.Mutex
	currentVersion := 5 // version the server will hand out on GET
	var getCount, putCount int
	var putVersions []int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			getCount++
			resp := domain.ExecutionResponse{
				ID:              42,
				ExecutionStatus: "PARTIAL",
				TradeType:       "BUY",
				Destination:     "ML",
				SecurityID:      "SEC1",
				Quantity:        100,
				QuantityFilled:  50,
				Version:         currentVersion,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case http.MethodPut:
			putCount++
			var req domain.ExecutionUpdateRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			putVersions = append(putVersions, req.Version)

			// The first PUT loses the optimistic-lock race: reject with 500 and
			// bump the server-side version so a stale re-PUT would keep losing.
			if putCount == 1 {
				currentVersion++
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"OptimisticLockingFailureException"}`))
				return
			}

			// Subsequent PUT must carry the fresh version to succeed.
			if req.Version != currentVersion {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"stale version"}`))
				return
			}
			resp := domain.ExecutionUpdateResponse{
				ID:              42,
				ExecutionStatus: "FULL",
				Version:         req.Version + 1,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := newTestExecutionClient(t, server.URL)

	resp, err := client.UpdateExecutionWithRefetch(context.Background(), 42,
		func(execution *domain.ExecutionResponse) (*domain.ExecutionUpdateRequest, error) {
			return &domain.ExecutionUpdateRequest{
				QuantityFilled: 100,
				AveragePrice:   10.0,
				Version:        execution.Version,
			}, nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	mu.Lock()
	defer mu.Unlock()

	// Two PUT attempts: the first conflicts, the second succeeds.
	assert.Equal(t, 2, putCount, "expected exactly two PUT attempts")
	// Each attempt re-fetched the version, so there is a GET per attempt.
	assert.Equal(t, 2, getCount, "expected a fresh GET before every PUT attempt")
	// The retry carried a different (fresher) version than the first attempt.
	require.Len(t, putVersions, 2)
	assert.Equal(t, 5, putVersions[0], "first PUT uses the initially fetched version")
	assert.Equal(t, 6, putVersions[1], "retry PUT uses the re-fetched, incremented version")
}

// TestUpdateExecutionWithRefetch_ValidationErrorAbortsBeforePut verifies that a
// non-retryable error from buildRequest (e.g. a business-rule validation failure)
// aborts the retry cycle without issuing a PUT.
func TestUpdateExecutionWithRefetch_ValidationErrorAbortsBeforePut(t *testing.T) {
	var mu sync.Mutex
	var getCount, putCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			getCount++
			resp := domain.ExecutionResponse{ID: 42, Version: 1}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case http.MethodPut:
			putCount++
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := newTestExecutionClient(t, server.URL)

	_, err := client.UpdateExecutionWithRefetch(context.Background(), 42,
		func(execution *domain.ExecutionResponse) (*domain.ExecutionUpdateRequest, error) {
			return nil, domain.NewValidationError("trade_type_mismatch", "bad")
		},
	)

	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, getCount, "validation abort should not retry the GET")
	assert.Equal(t, 0, putCount, "no PUT should be issued when buildRequest fails")
}
