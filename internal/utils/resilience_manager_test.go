package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestResilienceManager(t *testing.T) *ResilienceManager {
	t.Helper()

	appLogger, err := logger.New(logger.Config{
		Level:       "error",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	appMetrics := metrics.New(metrics.Config{Enabled: true, Namespace: "test"})

	return NewResilienceManager(ResilienceConfig{
		RetryConfig: RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  time.Millisecond,
			MaxDelay:      5 * time.Millisecond,
			BackoffFactor: 2.0,
		},
		CircuitBreakerConfig:  GetDefaultCircuitBreakerConfig("test-service"),
		DeadLetterQueueConfig: GetDefaultDeadLetterQueueConfig(),
		TimeoutConfig: TimeoutConfig{
			ExecutionServiceTimeout: 5 * time.Second,
			KafkaConsumerTimeout:    5 * time.Second,
			DefaultOperationTimeout: 5 * time.Second,
		},
	}, appLogger, appMetrics)
}

// TestExecuteWithResilience_SingleRetryCycleOnFailure verifies that a failing
// operation is retried exactly ONE full cycle (MaxAttempts invocations) and not two
// cycles, while still populating the dead letter queue on terminal failure.
func TestExecuteWithResilience_SingleRetryCycleOnFailure(t *testing.T) {
	rm := newTestResilienceManager(t)

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		return errors.New("temporary failure")
	}

	err := rm.ExecuteWithResilience(context.Background(), "test-op", fn, map[string]interface{}{"k": "v"})

	require.Error(t, err)
	// One retry cycle of MaxAttempts(3) invocations -- NOT two cycles (which would be 6).
	assert.Equal(t, 3, callCount, "operation should run exactly one retry cycle")
	// The terminal failure must still be recorded in the dead letter queue.
	assert.Equal(t, 1, len(rm.GetDeadLetterMessages()), "failed operation should be added to the DLQ once")
}

// TestExecuteWithResilience_SuccessRunsOnce verifies the happy path still runs the
// operation once and does not touch the dead letter queue.
func TestExecuteWithResilience_SuccessRunsOnce(t *testing.T) {
	rm := newTestResilienceManager(t)

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		return nil
	}

	err := rm.ExecuteWithResilience(context.Background(), "test-op", fn, map[string]interface{}{"k": "v"})

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
	assert.Equal(t, 0, len(rm.GetDeadLetterMessages()))
}
