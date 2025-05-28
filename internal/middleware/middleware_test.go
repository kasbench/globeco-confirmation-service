package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLogger(t *testing.T) {
	appLogger, err := logger.New(logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test",
	})
	require.NoError(t, err)

	middleware := RequestLogger(appLogger)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(logger.WithCorrelationIDContext(req.Context(), "test-correlation-id"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test response", w.Body.String())
}

func TestCorrelationID(t *testing.T) {
	middleware := CorrelationID()

	tests := []struct {
		name            string
		existingHeader  string
		expectGenerated bool
	}{
		{
			name:            "no existing correlation ID",
			existingHeader:  "",
			expectGenerated: true,
		},
		{
			name:            "existing correlation ID",
			existingHeader:  "existing-correlation-id",
			expectGenerated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				correlationID := logger.GetCorrelationID(r.Context())

				if tt.expectGenerated {
					assert.NotEmpty(t, correlationID)
					assert.NotEqual(t, tt.existingHeader, correlationID)
				} else {
					assert.Equal(t, tt.existingHeader, correlationID)
				}

				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.existingHeader != "" {
				req.Header.Set("X-Correlation-ID", tt.existingHeader)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			// Check that correlation ID is in response headers
			responseCorrelationID := w.Header().Get("X-Correlation-ID")
			assert.NotEmpty(t, responseCorrelationID)

			if !tt.expectGenerated {
				assert.Equal(t, tt.existingHeader, responseCorrelationID)
			}
		})
	}
}

func TestMetricsMiddleware(t *testing.T) {
	appMetrics := metrics.New(metrics.Config{
		Namespace: "test",
		Enabled:   true,
	})

	middleware := MetricsMiddleware(appMetrics)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate some processing time
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test response", w.Body.String())

	// Note: We can't easily test the metrics recording without exposing internal state
	// In a real scenario, you might want to use a metrics interface that can be mocked
}

func TestCORS(t *testing.T) {
	middleware := CORS()

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		checkHeaders   bool
	}{
		{
			name:           "OPTIONS request",
			method:         "OPTIONS",
			expectedStatus: http.StatusOK,
			checkHeaders:   true,
		},
		{
			name:           "GET request",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkHeaders:   true,
		},
		{
			name:           "POST request",
			method:         "POST",
			expectedStatus: http.StatusOK,
			checkHeaders:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("test response"))
				}
			}))

			req := httptest.NewRequest(tt.method, "/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkHeaders {
				assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
				assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
				assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
				assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "X-Correlation-ID")
				assert.Equal(t, "X-Correlation-ID", w.Header().Get("Access-Control-Expose-Headers"))
			}

			if tt.method == "OPTIONS" {
				assert.Empty(t, w.Body.String())
			} else {
				assert.Equal(t, "test response", w.Body.String())
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	middleware := SecurityHeaders()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test response", w.Body.String())

	// Check security headers
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "max-age=31536000; includeSubDomains", w.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "default-src 'self'", w.Header().Get("Content-Security-Policy"))
}

func TestRateLimiter(t *testing.T) {
	middleware := RateLimiter(2) // Allow 2 requests per second

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))

	// First request should succeed
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second request should succeed
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Third request should be rate limited
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "127.0.0.1:12345"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusTooManyRequests, w3.Code)
	assert.Contains(t, w3.Body.String(), "Rate limit exceeded")

	// Request from different IP should succeed
	req4 := httptest.NewRequest("GET", "/test", nil)
	req4.RemoteAddr = "127.0.0.2:12345"
	w4 := httptest.NewRecorder()
	handler.ServeHTTP(w4, req4)
	assert.Equal(t, http.StatusOK, w4.Code)
}

func TestResponseWriter(t *testing.T) {
	originalWriter := httptest.NewRecorder()
	wrapper := &responseWriter{
		ResponseWriter: originalWriter,
		statusCode:     http.StatusOK,
	}

	// Test WriteHeader
	wrapper.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, wrapper.statusCode)
	assert.Equal(t, http.StatusNotFound, originalWriter.Code)

	// Test Write
	data := []byte("test data")
	n, err := wrapper.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, "test data", originalWriter.Body.String())
}

func TestMiddlewareChaining(t *testing.T) {
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

	// Chain multiple middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := logger.GetCorrelationID(r.Context())
		assert.NotEmpty(t, correlationID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Apply middleware in reverse order (like chi does)
	chainedHandler := SecurityHeaders()(
		CORS()(
			MetricsMiddleware(appMetrics)(
				CorrelationID()(
					RequestLogger(appLogger)(handler),
				),
			),
		),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	chainedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test response", w.Body.String())

	// Check that all middleware applied their effects
	assert.NotEmpty(t, w.Header().Get("X-Correlation-ID"))
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}
