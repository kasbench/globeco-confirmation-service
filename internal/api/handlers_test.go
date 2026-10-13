package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing
type MockConfirmationService struct {
	mock.Mock
}

func (m *MockConfirmationService) IsHealthy(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *MockConfirmationService) GetStats() map[string]interface{} {
	args := m.Called()
	return args.Get(0).(map[string]interface{})
}

type MockKafkaConsumer struct {
	mock.Mock
}

func (m *MockKafkaConsumer) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockKafkaConsumer) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockKafkaConsumer) IsHealthy(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *MockKafkaConsumer) GetStats() map[string]interface{} {
	args := m.Called()
	return args.Get(0).(map[string]interface{})
}

func setupTestHandlers(t *testing.T) (*Handlers, *MockConfirmationService, *MockKafkaConsumer) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	appMetrics := metrics.New(metrics.Config{
		Namespace: "test",
		Enabled:   true,
	})

	mockConfirmationService := &MockConfirmationService{}
	mockKafkaConsumer := &MockKafkaConsumer{}

	handlers := NewHandlers(HandlerConfig{
		ConfirmationService: mockConfirmationService,
		KafkaConsumer:       mockKafkaConsumer,
		Logger:              appLogger,
		Metrics:             appMetrics,
	})

	return handlers, mockConfirmationService, mockKafkaConsumer
}

func TestNewHandlers(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	assert.NotNil(t, handlers)
	assert.NotNil(t, handlers.logger)
	assert.NotNil(t, handlers.metrics)
	assert.NotZero(t, handlers.startTime)
}

func TestLivenessHandler(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	req := httptest.NewRequest("GET", "/health/live", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.LivenessHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "UP", response.Status)
	assert.Equal(t, "globeco-confirmation-service", response.Service)
	assert.Equal(t, "1.0.0", response.Version)
	assert.Equal(t, "Service is alive and running", response.Message)
	assert.Equal(t, "test-correlation-id", response.RequestID)
	assert.NotZero(t, response.Timestamp)
	assert.NotEmpty(t, response.Uptime)
}

func TestReadinessHandler_Healthy(t *testing.T) {
	handlers, mockConfirmationService, mockKafkaConsumer := setupTestHandlers(t)

	// Mock healthy dependencies
	mockKafkaConsumer.On("IsHealthy", mock.AnythingOfType("*context.timerCtx")).Return(true)
	mockConfirmationService.On("IsHealthy", mock.AnythingOfType("*context.timerCtx")).Return(true)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.ReadinessHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "UP", response.Status)
	assert.Equal(t, "Service is ready to accept traffic", response.Message)
	assert.Contains(t, response.Checks, "kafka")
	assert.Contains(t, response.Checks, "execution_service")
	assert.Equal(t, "UP", response.Checks["kafka"].Status)
	assert.Equal(t, "UP", response.Checks["execution_service"].Status)

	mockKafkaConsumer.AssertExpectations(t)
	mockConfirmationService.AssertExpectations(t)
}

func TestReadinessHandler_Unhealthy(t *testing.T) {
	handlers, mockConfirmationService, mockKafkaConsumer := setupTestHandlers(t)

	// Mock unhealthy dependencies
	mockKafkaConsumer.On("IsHealthy", mock.AnythingOfType("*context.timerCtx")).Return(false)
	mockConfirmationService.On("IsHealthy", mock.AnythingOfType("*context.timerCtx")).Return(false)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.ReadinessHandler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "DOWN", response.Status)
	assert.Equal(t, "Service is not ready - dependency checks failed", response.Message)
	assert.Equal(t, "DOWN", response.Checks["kafka"].Status)
	assert.Equal(t, "DOWN", response.Checks["execution_service"].Status)

	mockKafkaConsumer.AssertExpectations(t)
	mockConfirmationService.AssertExpectations(t)
}

func TestReadinessHandler_PartiallyHealthy(t *testing.T) {
	handlers, mockConfirmationService, mockKafkaConsumer := setupTestHandlers(t)

	// Mock partially healthy dependencies (Kafka healthy, Execution Service unhealthy)
	mockKafkaConsumer.On("IsHealthy", mock.AnythingOfType("*context.timerCtx")).Return(true)
	mockConfirmationService.On("IsHealthy", mock.AnythingOfType("*context.timerCtx")).Return(false)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.ReadinessHandler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "DOWN", response.Status)
	assert.Equal(t, "UP", response.Checks["kafka"].Status)
	assert.Equal(t, "DOWN", response.Checks["execution_service"].Status)

	mockKafkaConsumer.AssertExpectations(t)
	mockConfirmationService.AssertExpectations(t)
}

func TestStatsHandler(t *testing.T) {
	handlers, mockConfirmationService, mockKafkaConsumer := setupTestHandlers(t)

	// Mock stats
	confirmationStats := map[string]interface{}{
		"service_name": "globeco-confirmation-service",
		"processed":    100,
	}
	kafkaStats := map[string]interface{}{
		"messages_consumed": 150,
		"lag":               5,
	}

	mockConfirmationService.On("GetStats").Return(confirmationStats)
	mockKafkaConsumer.On("GetStats").Return(kafkaStats)

	req := httptest.NewRequest("GET", "/stats", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.StatsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response StatsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "globeco-confirmation-service", response.Service)
	assert.Equal(t, "1.0.0", response.Version)
	assert.Equal(t, "development", response.Environment)
	assert.Equal(t, "test-correlation-id", response.RequestID)
	assert.Contains(t, response.Stats, "globeco-confirmation_service")
	assert.Contains(t, response.Stats, "kafka_consumer")
	assert.Contains(t, response.Stats, "runtime")

	mockConfirmationService.AssertExpectations(t)
	mockKafkaConsumer.AssertExpectations(t)
}

func TestVersionHandler(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	req := httptest.NewRequest("GET", "/version", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.VersionHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "globeco-confirmation-service", response["service"])
	assert.Equal(t, "1.0.0", response["version"])
	assert.Equal(t, "1.23.4", response["go_version"])
	assert.Equal(t, "test-correlation-id", response["request_id"])
	assert.NotNil(t, response["timestamp"])
	assert.NotNil(t, response["uptime"])
}

func TestRootHandler(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.RootHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "GlobeCo Confirmation Service", response["service"])
	assert.Equal(t, "1.0.0", response["version"])
	assert.Equal(t, "running", response["status"])
	assert.Equal(t, "test-correlation-id", response["request_id"])
	assert.Contains(t, response, "endpoints")

	endpoints := response["endpoints"].(map[string]interface{})
	assert.Equal(t, "/health/live", endpoints["health_live"])
	assert.Equal(t, "/health/ready", endpoints["health_ready"])
	assert.Equal(t, "/metrics", endpoints["metrics"])
	assert.Equal(t, "/stats", endpoints["stats"])
	assert.Equal(t, "/version", endpoints["version"])
}

func TestMetricsHandler(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	handler := handlers.MetricsHandler()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Prometheus metrics should be in text format
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
	// Should contain some metrics
	assert.Contains(t, w.Body.String(), "# HELP")
}

func TestWriteErrorResponse(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(context.Background(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handlers.writeErrorResponse(w, req, http.StatusBadRequest, "Test error message", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Bad Request", response.Error)
	assert.Equal(t, "Test error message", response.Message)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "test-correlation-id", response.RequestID)
	assert.NotZero(t, response.Timestamp)
}

func TestGetStatusString(t *testing.T) {
	assert.Equal(t, "UP", getStatusString(true))
	assert.Equal(t, "DOWN", getStatusString(false))
}

func TestGetEnvironment(t *testing.T) {
	env := getEnvironment()
	assert.Equal(t, "development", env)
}

// TestNoSuperfluousWriteHeaderWarning is a regression guard for the
// "http: superfluous response.WriteHeader call" flood. Handlers must not call
// w.WriteHeader(200) explicitly before writing a JSON body, because the otelhttp
// response wrapper (wired into NewRouter) forwards a second WriteHeader on the
// body write, which net/http logs as a superfluous call. This test drives the
// real router (so otelhttp is in the chain) through an httptest server whose
// ErrorLog is captured, and asserts no superfluous-WriteHeader warning is emitted
// for the steady-state endpoints. A future "logging cleanup" that re-adds the
// explicit WriteHeader(200) calls will fail here.
func TestNoSuperfluousWriteHeaderWarning(t *testing.T) {
	handlers, mockConfirmationService, mockKafkaConsumer := setupTestHandlers(t)

	// Readiness exercises both dependency checks; make them healthy so it returns 200.
	mockKafkaConsumer.On("IsHealthy", mock.Anything).Return(true)
	mockConfirmationService.On("IsHealthy", mock.Anything).Return(true)
	mockConfirmationService.On("GetStats").Return(map[string]interface{}{"ok": true})
	mockKafkaConsumer.On("GetStats").Return(map[string]interface{}{"ok": true})

	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)
	appMetrics := metrics.New(metrics.Config{Namespace: "test", Enabled: true})

	router := NewRouter(RouterConfig{
		Handlers: handlers,
		Logger:   appLogger,
		Metrics:  appMetrics,
	})

	// Capture the net/http server's ErrorLog, which is where the
	// "superfluous response.WriteHeader call" warning is written.
	var logBuf bytes.Buffer
	var mu sync.Mutex
	srv := httptest.NewServer(router)
	defer srv.Close()
	srv.Config.ErrorLog = log.New(&safeWriter{w: &logBuf, mu: &mu}, "", 0)

	paths := []string{"/health/live", "/health/ready", "/stats", "/version", "/"}
	for _, p := range paths {
		resp, err := http.Get(srv.URL + p)
		require.NoError(t, err, "GET %s", p)
		// Drain and close so the server finishes writing the response (and any
		// ErrorLog line) before we assert.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET %s should be 200", p)
	}

	mu.Lock()
	logged := logBuf.String()
	mu.Unlock()
	assert.NotContains(t, logged, "superfluous response.WriteHeader call",
		"handlers must not trigger the net/http superfluous WriteHeader warning; captured log:\n%s", logged)
}

// safeWriter serializes concurrent writes from the http server's ErrorLog.
type safeWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (s *safeWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
