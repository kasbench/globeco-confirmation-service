package utils

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"go.uber.org/zap"
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState int

const (
	// StateClosed - circuit breaker is closed, requests are allowed
	StateClosed CircuitBreakerState = iota
	// StateOpen - circuit breaker is open, requests are rejected
	StateOpen
	// StateHalfOpen - circuit breaker is half-open, limited requests are allowed
	StateHalfOpen
)

// String returns the string representation of the circuit breaker state
func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig represents circuit breaker configuration
type CircuitBreakerConfig struct {
	Name               string        // Name of the circuit breaker
	FailureThreshold   int           // Number of failures before opening
	SuccessThreshold   int           // Number of successes to close from half-open
	Timeout            time.Duration // Time to wait before transitioning to half-open
	MaxConcurrentCalls int           // Maximum concurrent calls in half-open state
	ResetTimeout       time.Duration // Time to reset failure count in closed state
}

// CircuitBreakerStats represents circuit breaker statistics
type CircuitBreakerStats struct {
	State                CircuitBreakerState
	FailureCount         int
	SuccessCount         int
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastFailureTime      time.Time
	LastSuccessTime      time.Time
	TotalRequests        int64
	TotalSuccesses       int64
	TotalFailures        int64
	TotalRejections      int64
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config  CircuitBreakerConfig
	state   CircuitBreakerState
	stats   CircuitBreakerStats
	mutex   sync.RWMutex
	logger  *logger.Logger
	metrics *metrics.Metrics

	// State transition tracking
	stateChangedAt time.Time
	halfOpenCalls  int
	lastResetTime  time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig, appLogger *logger.Logger, appMetrics *metrics.Metrics) *CircuitBreaker {
	// Set defaults
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 3
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxConcurrentCalls <= 0 {
		config.MaxConcurrentCalls = 1
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 60 * time.Second
	}

	cb := &CircuitBreaker{
		config:         config,
		state:          StateClosed,
		stateChangedAt: time.Now(),
		lastResetTime:  time.Now(),
		logger:         appLogger,
		metrics:        appMetrics,
	}

	// Initialize metrics
	if appMetrics != nil {
		appMetrics.SetCircuitBreakerState(config.Name, 0) // closed
	}

	return cb
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	// Check if we can execute
	if !cb.canExecute() {
		cb.recordRejection()
		return domain.NewCircuitBreakerError(
			fmt.Sprintf("circuit breaker %s is open", cb.config.Name),
		).WithCorrelationID(logger.GetCorrelationID(ctx))
	}

	// Execute the function
	err := fn(ctx)

	// Record the result
	if err != nil {
		cb.recordFailure(ctx, err)
	} else {
		cb.recordSuccess(ctx)
	}

	return err
}

// ExecuteWithResult executes a function with circuit breaker protection and returns a result
func (cb *CircuitBreaker) ExecuteWithResult(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}

	err := cb.Execute(ctx, func(ctx context.Context) error {
		var execErr error
		result, execErr = fn(ctx)
		return execErr
	})

	return result, err
}

// canExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) canExecute() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		// Reset failure count if enough time has passed
		if now.Sub(cb.lastResetTime) >= cb.config.ResetTimeout {
			cb.stats.ConsecutiveFailures = 0
			cb.lastResetTime = now
		}
		return true

	case StateOpen:
		// Check if we should transition to half-open
		if now.Sub(cb.stateChangedAt) >= cb.config.Timeout {
			cb.transitionToHalfOpen()
			return true
		}
		return false

	case StateHalfOpen:
		// Allow limited concurrent calls
		return cb.halfOpenCalls < cb.config.MaxConcurrentCalls

	default:
		return false
	}
}

// recordSuccess records a successful execution
func (cb *CircuitBreaker) recordSuccess(ctx context.Context) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.stats.TotalRequests++
	cb.stats.TotalSuccesses++
	cb.stats.SuccessCount++
	cb.stats.ConsecutiveSuccesses++
	cb.stats.ConsecutiveFailures = 0
	cb.stats.LastSuccessTime = time.Now()

	if cb.metrics != nil {
		cb.metrics.RecordCircuitBreakerOperation(cb.config.Name, "success")
	}

	switch cb.state {
	case StateHalfOpen:
		cb.halfOpenCalls--
		if cb.stats.ConsecutiveSuccesses >= cb.config.SuccessThreshold {
			cb.transitionToClosed(ctx)
		}

	case StateClosed:
		// Reset failure count on success
		cb.stats.ConsecutiveFailures = 0
	}
}

// recordFailure records a failed execution
func (cb *CircuitBreaker) recordFailure(ctx context.Context, err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.stats.TotalRequests++
	cb.stats.TotalFailures++
	cb.stats.FailureCount++
	cb.stats.ConsecutiveFailures++
	cb.stats.ConsecutiveSuccesses = 0
	cb.stats.LastFailureTime = time.Now()

	if cb.metrics != nil {
		cb.metrics.RecordCircuitBreakerOperation(cb.config.Name, "failure")
	}

	switch cb.state {
	case StateClosed:
		if cb.stats.ConsecutiveFailures >= cb.config.FailureThreshold {
			cb.transitionToOpen(ctx)
		}

	case StateHalfOpen:
		cb.halfOpenCalls--
		cb.transitionToOpen(ctx)
	}
}

// recordRejection records a rejected request
func (cb *CircuitBreaker) recordRejection() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.stats.TotalRejections++

	if cb.metrics != nil {
		cb.metrics.RecordCircuitBreakerOperation(cb.config.Name, "rejection")
	}
}

// transitionToClosed transitions the circuit breaker to closed state
func (cb *CircuitBreaker) transitionToClosed(ctx context.Context) {
	cb.state = StateClosed
	cb.stateChangedAt = time.Now()
	cb.lastResetTime = time.Now()
	cb.halfOpenCalls = 0
	cb.stats.ConsecutiveFailures = 0

	if cb.metrics != nil {
		cb.metrics.SetCircuitBreakerState(cb.config.Name, 0) // closed
	}

	cb.logger.WithContext(ctx).Info("Circuit breaker transitioned to closed",
		zap.String("circuit_breaker", cb.config.Name),
		zap.String("previous_state", "half-open"),
		zap.Int("consecutive_successes", cb.stats.ConsecutiveSuccesses),
	)
}

// transitionToOpen transitions the circuit breaker to open state
func (cb *CircuitBreaker) transitionToOpen(ctx context.Context) {
	previousState := cb.state.String()
	cb.state = StateOpen
	cb.stateChangedAt = time.Now()
	cb.halfOpenCalls = 0

	if cb.metrics != nil {
		cb.metrics.SetCircuitBreakerState(cb.config.Name, 1) // open
	}

	cb.logger.WithContext(ctx).Warn("Circuit breaker transitioned to open",
		zap.String("circuit_breaker", cb.config.Name),
		zap.String("previous_state", previousState),
		zap.Int("consecutive_failures", cb.stats.ConsecutiveFailures),
		zap.Duration("timeout", cb.config.Timeout),
	)
}

// transitionToHalfOpen transitions the circuit breaker to half-open state
func (cb *CircuitBreaker) transitionToHalfOpen() {
	cb.state = StateHalfOpen
	cb.stateChangedAt = time.Now()
	cb.halfOpenCalls = 0
	cb.stats.ConsecutiveSuccesses = 0

	if cb.metrics != nil {
		cb.metrics.SetCircuitBreakerState(cb.config.Name, 2) // half-open
	}

	// Note: We don't log here as this is called within a lock and we don't have context
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetStats returns the current statistics of the circuit breaker
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.stats
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset(ctx context.Context) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	previousState := cb.state.String()
	cb.state = StateClosed
	cb.stateChangedAt = time.Now()
	cb.lastResetTime = time.Now()
	cb.halfOpenCalls = 0
	cb.stats.ConsecutiveFailures = 0
	cb.stats.ConsecutiveSuccesses = 0

	if cb.metrics != nil {
		cb.metrics.SetCircuitBreakerState(cb.config.Name, 0) // closed
	}

	cb.logger.WithContext(ctx).Info("Circuit breaker manually reset",
		zap.String("circuit_breaker", cb.config.Name),
		zap.String("previous_state", previousState),
	)
}

// GetDefaultCircuitBreakerConfig returns a default circuit breaker configuration
func GetDefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:               name,
		FailureThreshold:   5,
		SuccessThreshold:   3,
		Timeout:            30 * time.Second,
		MaxConcurrentCalls: 1,
		ResetTimeout:       60 * time.Second,
	}
}
