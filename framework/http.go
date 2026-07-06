// Package framework HTTP server enhancements provide production-ready HTTP endpoints.
//
// The enhanced HTTP server includes:
//   - Health endpoints (/health, /health/ready, /health/live)
//   - Metrics endpoint (/metrics) with Prometheus integration
//   - Debug endpoints (/debug/pprof/*) when enabled
//   - Request logging middleware
//   - CORS support with configuration
//   - Graceful shutdown handling
//
// All endpoints are configurable and can be enabled/disabled based on environment.

package framework

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// HTTPServerConfig contains configuration for the HTTP server endpoints.
type HTTPServerConfig struct {
	// CORS configuration
	EnableCORS      bool     `yaml:"enable_cors" env:"ENABLE_CORS"`
	CORSOrigins     []string `yaml:"cors_origins" env:"CORS_ORIGINS"`
	CORSMethods     []string `yaml:"cors_methods" env:"CORS_METHODS"`
	CORSHeaders     []string `yaml:"cors_headers" env:"CORS_HEADERS"`
	CORSCredentials bool     `yaml:"cors_credentials" env:"CORS_CREDENTIALS"`

	// Endpoint configuration
	EnableMetrics    bool   `yaml:"enable_metrics" env:"ENABLE_METRICS"`
	MetricsPath      string `yaml:"metrics_path" env:"METRICS_PATH"`
	HealthPathPrefix string `yaml:"health_path_prefix" env:"HEALTH_PATH_PREFIX"`

	// Request logging
	EnableRequestLogging bool `yaml:"enable_request_logging" env:"ENABLE_REQUEST_LOGGING"`
}

// DefaultHTTPServerConfig returns HTTP server configuration with sensible defaults.
func DefaultHTTPServerConfig() HTTPServerConfig {
	return HTTPServerConfig{
		EnableCORS:           false,
		CORSOrigins:          []string{}, // Empty by default for security
		CORSMethods:          []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		CORSHeaders:          []string{"Content-Type", "Authorization"},
		CORSCredentials:      false,
		EnableMetrics:        true,
		MetricsPath:          "/metrics",
		HealthPathPrefix:     "/health",
		EnableRequestLogging: true,
	}
}

// Validate performs comprehensive validation on HTTP server configuration.
func (c *HTTPServerConfig) Validate() error {
	// Validate CORS configuration
	if c.EnableCORS {
		if len(c.CORSOrigins) == 0 {
			return fmt.Errorf("CORS enabled but no origins specified - this will block all cross-origin requests")
		}

		// Check for dangerous wildcard + credentials combination
		hasWildcard := false
		for _, origin := range c.CORSOrigins {
			if origin == "*" {
				hasWildcard = true
				break
			}
		}

		if hasWildcard && c.CORSCredentials {
			return fmt.Errorf("SECURITY: cannot use wildcard CORS origin (*) with credentials enabled - this is a security vulnerability")
		}

		// Validate methods
		if len(c.CORSMethods) == 0 {
			return fmt.Errorf("CORS enabled but no methods specified")
		}
	}

	// Validate paths
	if c.MetricsPath == "" {
		return fmt.Errorf("metrics_path cannot be empty when metrics are enabled")
	}
	if c.HealthPathPrefix == "" {
		return fmt.Errorf("health_path_prefix cannot be empty")
	}

	// Ensure paths don't conflict
	if c.MetricsPath == c.HealthPathPrefix {
		return fmt.Errorf("metrics_path and health_path_prefix cannot be the same")
	}

	return nil
}

// HTTPServerBuilder builds an enhanced HTTP server with configurable endpoints.
type HTTPServerBuilder struct {
	config   HTTPServerConfig
	mux      *http.ServeMux
	app      *App
	logger   zerolog.Logger
	registry prometheus.Registerer
}

// NewHTTPServerBuilder creates a new HTTP server builder.
func NewHTTPServerBuilder(app *App, config HTTPServerConfig) *HTTPServerBuilder {
	return &HTTPServerBuilder{
		config:   config,
		mux:      http.NewServeMux(),
		app:      app,
		logger:   app.logging.WithService(app.config.ServiceName, "http"),
		registry: prometheus.DefaultRegisterer,
	}
}

// RegisterCustomRoutes registers custom HTTP routes with the server.
func (b *HTTPServerBuilder) RegisterCustomRoutes(registerFunc func(*http.ServeMux)) {
	registerFunc(b.mux)
}

// Build constructs the HTTP server with all configured endpoints and middleware.
func (b *HTTPServerBuilder) Build() (*http.Server, error) {
	// Validate configuration before building
	if err := b.config.Validate(); err != nil {
		return nil, fmt.Errorf("HTTP server configuration validation failed: %w", err)
	}

	// Apply middleware stack
	handler := b.buildHandler()

	return &http.Server{
		Addr:         b.app.config.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  b.app.config.HTTPReadTimeout,
		WriteTimeout: b.app.config.HTTPWriteTimeout,
		IdleTimeout:  b.app.config.HTTPIdleTimeout,
	}, nil
}

// buildHandler creates the complete HTTP handler with all endpoints and middleware.
func (b *HTTPServerBuilder) buildHandler() http.Handler {
	// Register health endpoints
	b.registerHealthEndpoints()

	// Register metrics endpoint if enabled
	if b.config.EnableMetrics {
		b.registerMetricsEndpoint()
	}

	// Register pprof endpoints if enabled
	if b.app.config.EnablePprof {
		b.registerPprofEndpoints()
	}

	// Apply middleware stack
	handler := http.Handler(b.mux)

	// Request logging middleware
	if b.config.EnableRequestLogging {
		handler = b.requestLoggingMiddleware(handler)
	}

	// CORS middleware
	if b.config.EnableCORS {
		handler = b.corsMiddleware(handler)
	}

	return handler
}

// registerHealthEndpoints registers the health check endpoints.
func (b *HTTPServerBuilder) registerHealthEndpoints() {
	prefix := b.config.HealthPathPrefix

	b.mux.HandleFunc(prefix, b.app.handleHealth)
	b.mux.HandleFunc(prefix+"/ready", b.app.handleReady)
	b.mux.HandleFunc(prefix+"/live", b.app.handleLive)
}

// registerMetricsEndpoint registers the Prometheus metrics endpoint.
func (b *HTTPServerBuilder) registerMetricsEndpoint() {
	path := b.config.MetricsPath
	if path == "" {
		path = "/metrics"
	}

	// Create Prometheus handler with security considerations
	handler := promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
			Timeout:           30 * time.Second,
			ErrorLog:          &promLogAdapter{logger: b.logger},
		},
	)

	b.mux.Handle(path, handler)

	// Security consideration logging
	if b.app.config.IsProduction() {
		b.logger.Warn().
			Str("path", path).
			Str("environment", b.app.config.AppEnv).
			Msg("Metrics endpoint registered in production - ensure proper network security and access controls")
	} else {
		b.logger.Info().
			Str("path", path).
			Str("environment", b.app.config.AppEnv).
			Msg("Metrics endpoint registered")
	}
}

// registerPprofEndpoints registers debug/pprof endpoints when enabled.
func (b *HTTPServerBuilder) registerPprofEndpoints() {
	// SECURITY: Never enable pprof in production
	if b.app.config.IsProduction() {
		b.logger.Error().
			Str("environment", b.app.config.AppEnv).
			Msg("SECURITY ERROR: Pprof endpoints cannot be enabled in production environment")
		return
	}

	// Additional warning for non-development environments
	if !b.app.config.IsDevelopment() {
		b.logger.Warn().
			Str("environment", b.app.config.AppEnv).
			Msg("WARNING: Pprof endpoints enabled in non-development environment - ensure network security")
	}

	b.mux.HandleFunc("/debug/pprof/", pprof.Index)
	b.mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	b.mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	b.mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	b.mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	b.logger.Info().
		Str("environment", b.app.config.AppEnv).
		Msg("Debug pprof endpoints registered - available at /debug/pprof/")
}

// requestLoggingMiddleware logs HTTP requests with structured logging.
func (b *HTTPServerBuilder) requestLoggingMiddleware(next http.Handler) http.Handler {
	// Pre-check if logging is enabled to avoid middleware overhead
	logLevel := b.logger.GetLevel()
	if logLevel > zerolog.InfoLevel {
		return next // Skip middleware entirely if not logging at info level
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response writer wrapper to capture status code
		wrapper := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Process request
		next.ServeHTTP(wrapper, r)

		// Only log if we're still at appropriate log level (may have changed during request)
		if b.logger.GetLevel() <= zerolog.InfoLevel {
			b.logHTTPRequest(r, wrapper, time.Since(start))
		}
	})
}

// logHTTPRequest logs HTTP request details with optimized field construction.
func (b *HTTPServerBuilder) logHTTPRequest(r *http.Request, wrapper *responseWriter, duration time.Duration) {
	// Determine log level based on status code
	var event *zerolog.Event
	statusCode := wrapper.statusCode

	if statusCode >= 500 {
		event = b.logger.Error()
	} else if statusCode >= 400 {
		event = b.logger.Warn()
	} else {
		event = b.logger.Info()
	}

	// Core request fields (always included)
	event = event.
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("remote_addr", r.RemoteAddr).
		Int("status_code", statusCode).
		Dur("duration", duration)

	// Optional fields (only if present to reduce allocations)
	if r.ContentLength > 0 {
		event = event.Int64("content_length", r.ContentLength)
	}

	if r.URL.RawQuery != "" {
		event = event.Str("query", r.URL.RawQuery)
	}

	userAgent := r.UserAgent()
	if userAgent != "" && userAgent != "-" {
		event = event.Str("user_agent", userAgent)
	}

	// Log with appropriate message based on status code
	if statusCode >= 500 {
		event.Msg("HTTP request completed with server error")
	} else if statusCode >= 400 {
		event.Msg("HTTP request completed with client error")
	} else {
		event.Msg("HTTP request completed successfully")
	}
}

// corsMiddleware adds CORS headers based on configuration.
func (b *HTTPServerBuilder) corsMiddleware(next http.Handler) http.Handler {
	// Wildcard+credentials safety is enforced by config Validate() at startup,
	// so it is not re-checked on every request. Header values that don't vary
	// per-request are joined once here rather than on every call.
	hasWildcard := len(b.config.CORSOrigins) == 1 && b.config.CORSOrigins[0] == "*"
	allowMethods := strings.Join(b.config.CORSMethods, ", ")
	allowHeaders := strings.Join(b.config.CORSHeaders, ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Set CORS headers safely
		if b.isAllowedOrigin(origin) {
			if hasWildcard && !b.config.CORSCredentials {
				// Safe to use wildcard only when credentials are disabled
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				// Use specific origin when credentials are enabled or specific origins are configured
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", allowMethods)
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)

		if b.config.CORSCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isAllowedOrigin checks if the given origin is allowed by CORS configuration.
func (b *HTTPServerBuilder) isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}

	for _, allowedOrigin := range b.config.CORSOrigins {
		if allowedOrigin == "*" || allowedOrigin == origin {
			return true
		}
	}

	return false
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write ensures status code is set if WriteHeader wasn't called.
func (rw *responseWriter) Write(data []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(data)
}

// promLogAdapter adapts zerolog.Logger to the log.Logger interface required by Prometheus.
type promLogAdapter struct {
	logger zerolog.Logger
}

// Println implements the log.Logger interface for Prometheus error logging.
func (p *promLogAdapter) Println(v ...interface{}) {
	p.logger.Error().Interface("prometheus_error", v).Msg("Prometheus metrics error")
}
