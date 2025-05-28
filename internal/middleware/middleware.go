package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
	"go.uber.org/zap"
)

// RequestLogger creates a middleware that logs HTTP requests
func RequestLogger(appLogger *logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response writer wrapper to capture status code
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Log the incoming request
			appLogger.WithContext(r.Context()).Info("HTTP request started",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
			)

			// Process the request
			next.ServeHTTP(ww, r)

			// Log the completed request
			duration := time.Since(start)
			appLogger.WithContext(r.Context()).Info("HTTP request completed",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status_code", ww.statusCode),
				zap.Duration("duration", duration),
				zap.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// CorrelationID creates a middleware that adds correlation ID to the request context
func CorrelationID() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate or extract correlation ID
			correlationID := logger.GenerateCorrelationID()

			// Check if correlation ID is provided in headers
			if headerID := r.Header.Get("X-Correlation-ID"); headerID != "" {
				correlationID = headerID
			}

			// Add correlation ID to response headers
			w.Header().Set("X-Correlation-ID", correlationID)

			// Add correlation ID to request context
			ctx := logger.WithCorrelationIDContext(r.Context(), correlationID)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// MetricsMiddleware creates a middleware that records HTTP metrics
func MetricsMiddleware(appMetrics *metrics.Metrics) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response writer wrapper to capture status code
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Increment in-flight requests
			appMetrics.IncAPICallsInFlight()
			defer appMetrics.DecAPICallsInFlight()

			// Process the request
			next.ServeHTTP(ww, r)

			// Record metrics
			duration := time.Since(start)
			statusCode := strconv.Itoa(ww.statusCode)
			appMetrics.RecordAPICall(r.Method, r.URL.Path, statusCode, duration)
		})
	}
}

// CORS creates a middleware that handles Cross-Origin Resource Sharing
func CORS() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Correlation-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Correlation-ID")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders creates a middleware that adds security headers
func SecurityHeaders() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add security headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter creates a simple rate limiting middleware
func RateLimiter(requestsPerSecond int) func(next http.Handler) http.Handler {
	// Simple in-memory rate limiter (for production, use Redis or similar)
	limiter := make(map[string][]time.Time)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := r.RemoteAddr
			now := time.Now()

			// Clean old entries
			if requests, exists := limiter[clientIP]; exists {
				var validRequests []time.Time
				for _, reqTime := range requests {
					if now.Sub(reqTime) < time.Second {
						validRequests = append(validRequests, reqTime)
					}
				}
				limiter[clientIP] = validRequests
			}

			// Check rate limit
			if len(limiter[clientIP]) >= requestsPerSecond {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Add current request
			limiter[clientIP] = append(limiter[clientIP], now)

			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
