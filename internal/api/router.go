package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	custommiddleware "github.com/kasbench/globeco-confirmation-service/internal/middleware"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"github.com/kasbench/globeco-confirmation-service/pkg/metrics"
)

// RouterConfig represents the configuration for the HTTP router
type RouterConfig struct {
	Handlers *Handlers
	Logger   *logger.Logger
	Metrics  *metrics.Metrics
}

// NewRouter creates a new HTTP router with all endpoints and middleware configured
func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Add built-in middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Add custom middleware
	if config.Logger != nil {
		r.Use(custommiddleware.RequestLogger(config.Logger))
		r.Use(custommiddleware.CorrelationID())
	}

	if config.Metrics != nil {
		r.Use(custommiddleware.MetricsMiddleware(config.Metrics))
	}

	// Add CORS middleware for development
	r.Use(custommiddleware.CORS())

	// Health check endpoints (required by Kubernetes)
	r.Route("/health", func(r chi.Router) {
		r.Get("/live", config.Handlers.LivenessHandler)
		r.Get("/ready", config.Handlers.ReadinessHandler)
	})

	// Metrics endpoint for Prometheus
	r.Handle("/metrics", config.Handlers.MetricsHandler())

	// Operational endpoints
	r.Get("/stats", config.Handlers.StatsHandler)
	r.Get("/version", config.Handlers.VersionHandler)

	// Root endpoint
	r.Get("/", config.Handlers.RootHandler)

	// Add a catch-all for undefined routes
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		config.Handlers.writeErrorResponse(w, r, http.StatusNotFound, "Endpoint not found", nil)
	})

	// Add method not allowed handler
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		config.Handlers.writeErrorResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed", nil)
	})

	return r
}
