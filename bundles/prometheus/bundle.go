// Package prometheus provides Prometheus metrics integration bundle for Forge applications.
//
// The Prometheus bundle provides:
//   - Automatic application metrics (HTTP requests, gRPC calls, errors)
//   - Custom metrics registration and collection
//   - Integration with existing framework components
//   - Health check metrics and monitoring
//   - Connection pool and database metrics
//   - JWT authentication metrics
//   - Circuit breaker and retry metrics
//   - Grafana dashboard configuration
//
// # Basic Usage
//
// Add the Prometheus bundle to your application:
//
//	config := prometheus.Config{
//		Namespace: "myservice",
//		Subsystem: "api",
//		EnableDefaultMetrics: true,
//		EnableHTTPMetrics: true,
//		EnableGRPCMetrics: true,
//	}
//
//	bundle := prometheus.NewBundle(config)
//
//	app, err := framework.New(
//		framework.WithConfig(&baseConfig),
//		framework.WithBundle(bundle),
//	)
//
// # Custom Metrics
//
// Register and use custom metrics:
//
//	// Get metrics registry
//	registry := bundle.Registry()
//
//	// Create custom counter
//	userCreated := prometheus.NewCounterVec(
//		prometheus.CounterOpts{
//			Namespace: "myservice",
//			Name:      "users_created_total",
//			Help:      "Total number of users created",
//		},
//		[]string{"source"},
//	)
//	registry.MustRegister(userCreated)
//
//	// Use in your code
//	userCreated.WithLabelValues("api").Inc()
//
// # Automatic Metrics
//
// The bundle automatically collects:
//   - HTTP request metrics (duration, count, status codes)
//   - gRPC call metrics (duration, count, status codes)
//   - Health check metrics (success/failure, duration)
//   - Database connection pool metrics
//   - Redis connection pool metrics
//   - JWT authentication metrics
//   - Circuit breaker state metrics
//
// # Integration with Other Bundles
//
// The Prometheus bundle automatically integrates with:
//   - PostgreSQL bundle for database metrics
//   - Redis bundle for cache and connection metrics
//   - JWT bundle for authentication metrics
//   - HTTP client bundle for request metrics
//
// # Grafana Integration
//
// The bundle provides pre-built Grafana dashboards for:
//   - Application overview and health
//   - HTTP and gRPC request monitoring
//   - Database and cache performance
//   - Error rates and SLA monitoring
package prometheus

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/datariot/forge/errors"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// Config contains Prometheus metrics configuration.
type Config struct {
	// Namespace for all metrics (typically service name)
	Namespace string

	// Subsystem for metrics grouping (optional)
	Subsystem string

	// Metric collection configuration
	EnableDefaultMetrics bool // Enable Go runtime metrics (memory, GC, etc.)
	EnableHTTPMetrics    bool // Enable HTTP request/response metrics
	EnableGRPCMetrics    bool // Enable gRPC call metrics
	EnableHealthMetrics  bool // Enable health check metrics
	EnableBundleMetrics  bool // Enable bundle-specific metrics (DB, Redis, etc.)

	// Custom registry (optional - uses global registry if nil)
	Registry prometheus.Registerer

	// Metric label configuration
	ServiceLabels map[string]string // Additional labels for all metrics

	// Performance configuration
	HistogramBuckets []float64 // Custom histogram buckets for latency metrics
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		EnableDefaultMetrics: true,
		EnableHTTPMetrics:    true,
		EnableGRPCMetrics:    true,
		EnableHealthMetrics:  true,
		EnableBundleMetrics:  true,
		ServiceLabels:        make(map[string]string),
		// Default histogram buckets for latency (in seconds)
		HistogramBuckets: []float64{
			0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
		},
	}
}

// Validate validates the Prometheus configuration.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return stderrors.New("namespace is required for Prometheus metrics")
	}

	// Validate metric naming (Prometheus has strict naming rules)
	if !isValidMetricName(c.Namespace) {
		return stderrors.New("invalid namespace format: must match [a-zA-Z_:][a-zA-Z0-9_:]*")
	}

	if c.Subsystem != "" && !isValidMetricName(c.Subsystem) {
		return stderrors.New("invalid subsystem format: must match [a-zA-Z_:][a-zA-Z0-9_:]*")
	}

	// Validate service labels for security and cardinality
	if len(c.ServiceLabels) > 10 {
		return fmt.Errorf("too many service labels (%d), maximum 10 allowed to prevent high cardinality", len(c.ServiceLabels))
	}

	for key, value := range c.ServiceLabels {
		if !isValidLabelName(key) {
			return fmt.Errorf("invalid service label name: %s", key)
		}
		if len(value) > 256 {
			return fmt.Errorf("service label value too long for key %s (maximum 256 characters)", key)
		}
		// Prevent sensitive data in labels
		if strings.Contains(strings.ToLower(key), "password") ||
			strings.Contains(strings.ToLower(key), "secret") ||
			strings.Contains(strings.ToLower(key), "token") {
			return fmt.Errorf("service label %s appears to contain sensitive data", key)
		}
	}

	// Validate histogram buckets
	if len(c.HistogramBuckets) == 0 {
		return stderrors.New("histogram buckets cannot be empty")
	}

	if len(c.HistogramBuckets) > 50 {
		return fmt.Errorf("too many histogram buckets (%d), maximum 50 allowed for performance", len(c.HistogramBuckets))
	}

	// Ensure buckets are in ascending order and reasonable
	for i := 1; i < len(c.HistogramBuckets); i++ {
		if c.HistogramBuckets[i] <= c.HistogramBuckets[i-1] {
			return stderrors.New("histogram buckets must be in ascending order")
		}
	}

	// Validate bucket ranges are reasonable
	if c.HistogramBuckets[0] <= 0 {
		return stderrors.New("histogram buckets must be positive")
	}

	return nil
}

// Bundle provides Prometheus metrics integration for Forge applications.
type Bundle struct {
	config   Config
	registry prometheus.Registerer
	gatherer prometheus.Gatherer
	logger   zerolog.Logger

	// Application metrics
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	grpcRequestsTotal   *prometheus.CounterVec
	grpcRequestDuration *prometheus.HistogramVec
	healthCheckDuration *prometheus.HistogramVec
	healthCheckTotal    *prometheus.CounterVec

	// Bundle integration metrics
	dbConnectionsActive    prometheus.Gauge
	dbConnectionsIdle      prometheus.Gauge
	redisConnectionsActive prometheus.Gauge
	redisConnectionsIdle   prometheus.Gauge
	jwtTokensValidated     *prometheus.CounterVec
	circuitBreakerState    *prometheus.GaugeVec
}

// NewBundle creates a new Prometheus metrics bundle.
func NewBundle(config Config) *Bundle {
	return &Bundle{
		config: config,
	}
}

// Name returns the bundle name.
func (b *Bundle) Name() string {
	return "prometheus"
}

// Initialize sets up Prometheus metrics collection.
func (b *Bundle) Initialize(app *framework.App) error {
	if err := b.config.Validate(); err != nil {
		return errors.ErrInvalidConfiguration.WithMessage("Prometheus configuration validation failed").WithCause(err)
	}

	b.logger = app.Logger().WithService("prometheus", "prometheus")

	// Use provided registry or create new one
	if b.config.Registry != nil {
		b.registry = b.config.Registry
		if gatherer, ok := b.config.Registry.(prometheus.Gatherer); ok {
			b.gatherer = gatherer
		} else {
			b.gatherer = prometheus.DefaultGatherer
		}
	} else {
		b.registry = prometheus.DefaultRegisterer
		b.gatherer = prometheus.DefaultGatherer
	}

	// Initialize metrics
	if err := b.initializeMetrics(); err != nil {
		return fmt.Errorf("failed to initialize Prometheus metrics: %w", err)
	}

	// Register default Go metrics if enabled
	if b.config.EnableDefaultMetrics {
		b.registry.MustRegister(collectors.NewGoCollector())
		b.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	}

	// Wire the bundle's own interceptor/middleware into the framework so
	// RecordHTTPRequest/RecordGRPCRequest are called automatically for every
	// request, without requiring per-handler instrumentation. Manual calls to
	// RecordHTTPRequest/RecordGRPCRequest remain supported alongside this.
	if b.config.EnableGRPCMetrics {
		app.AddUnaryInterceptor(b.UnaryServerInterceptor())
		app.AddStreamInterceptor(b.StreamServerInterceptor())
	}
	if b.config.EnableHTTPMetrics {
		app.AddHTTPMiddleware(b.HTTPMiddleware())
	}

	return nil
}

// UnaryServerInterceptor returns a gRPC unary server interceptor that
// automatically records RecordGRPCRequest metrics for every unary call. The
// status label is derived from the returned error via gRPC status codes
// (e.g. "OK", "NotFound", "Internal"), so a nil error always records "OK".
func (b *Bundle) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		b.RecordGRPCRequest(info.FullMethod, status.Code(err).String(), time.Since(start))
		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor that
// automatically records RecordGRPCRequest metrics for every streaming call.
// See UnaryServerInterceptor for how the status label is derived.
func (b *Bundle) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		b.RecordGRPCRequest(info.FullMethod, status.Code(err).String(), time.Since(start))
		return err
	}
}

// HTTPMiddleware returns HTTP middleware that automatically records
// RecordHTTPRequest metrics for every request.
//
// The endpoint label prefers r.Pattern, the matched net/http.ServeMux
// pattern (e.g. "GET /users/{id}"), which the framework's mux populates
// before invoking the handler. That keeps the label a bounded route
// template rather than raw concrete paths. It falls back to r.URL.Path only
// for requests that matched no route (e.g. 404s), where cardinality is
// naturally limited by what clients actually request.
func (b *Bundle) HTTPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapper := &statusCapturingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapper, r)

			endpoint := r.Pattern
			if endpoint == "" {
				endpoint = r.URL.Path
			}
			b.RecordHTTPRequest(r.Method, endpoint, wrapper.statusCode, time.Since(start))
		})
	}
}

// statusCapturingResponseWriter wraps http.ResponseWriter to capture the
// final status code for metrics. framework/http.go has an equivalent
// responseWriter for its request-logging middleware, but it is unexported
// and package-private, so this is a small local copy rather than a shared
// dependency across module boundaries.
type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before delegating.
func (w *statusCapturingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// Write ensures a status code is recorded even if WriteHeader was never
// called explicitly (the standard library implicitly writes 200 OK).
func (w *statusCapturingResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

// Registry returns the Prometheus registry for custom metric registration.
func (b *Bundle) Registry() prometheus.Registerer {
	return b.registry
}

// Gatherer returns the Prometheus gatherer for metric collection.
func (b *Bundle) Gatherer() prometheus.Gatherer {
	return b.gatherer
}

// initializeMetrics creates and registers all automatic metrics.
func (b *Bundle) initializeMetrics() error {
	// HTTP metrics
	if b.config.EnableHTTPMetrics {
		b.httpRequestsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests processed",
			},
			[]string{"method", "endpoint", "status_code"},
		)

		b.httpRequestDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   b.config.HistogramBuckets,
			},
			[]string{"method", "endpoint"},
		)

		if err := b.registerMetric(b.httpRequestsTotal); err != nil {
			return err
		}
		if err := b.registerMetric(b.httpRequestDuration); err != nil {
			return err
		}
	}

	// gRPC metrics
	if b.config.EnableGRPCMetrics {
		b.grpcRequestsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "grpc_requests_total",
				Help:      "Total number of gRPC requests processed",
			},
			[]string{"method", "status_code"},
		)

		b.grpcRequestDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "grpc_request_duration_seconds",
				Help:      "gRPC request duration in seconds",
				Buckets:   b.config.HistogramBuckets,
			},
			[]string{"method"},
		)

		if err := b.registerMetric(b.grpcRequestsTotal); err != nil {
			return err
		}
		if err := b.registerMetric(b.grpcRequestDuration); err != nil {
			return err
		}
	}

	// Health check metrics
	if b.config.EnableHealthMetrics {
		b.healthCheckTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "health_checks_total",
				Help:      "Total number of health checks performed",
			},
			[]string{"check_name", "check_type", "status"},
		)

		b.healthCheckDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "health_check_duration_seconds",
				Help:      "Health check duration in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"check_name", "check_type"},
		)

		if err := b.registerMetric(b.healthCheckTotal); err != nil {
			return err
		}
		if err := b.registerMetric(b.healthCheckDuration); err != nil {
			return err
		}
	}

	// Bundle integration metrics
	if b.config.EnableBundleMetrics {
		b.dbConnectionsActive = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "database_connections_active",
				Help:      "Number of active database connections",
			},
		)

		b.dbConnectionsIdle = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "database_connections_idle",
				Help:      "Number of idle database connections",
			},
		)

		b.redisConnectionsActive = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "redis_connections_active",
				Help:      "Number of active Redis connections",
			},
		)

		b.redisConnectionsIdle = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "redis_connections_idle",
				Help:      "Number of idle Redis connections",
			},
		)

		b.jwtTokensValidated = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "jwt_tokens_validated_total",
				Help:      "Total number of JWT tokens validated",
			},
			[]string{"status", "service"},
		)

		b.circuitBreakerState = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: b.config.Namespace,
				Subsystem: b.config.Subsystem,
				Name:      "circuit_breaker_state",
				Help:      "Circuit breaker state (0=closed, 1=half-open, 2=open)",
			},
			[]string{"name"},
		)

		metrics := []prometheus.Collector{
			b.dbConnectionsActive,
			b.dbConnectionsIdle,
			b.redisConnectionsActive,
			b.redisConnectionsIdle,
			b.jwtTokensValidated,
			b.circuitBreakerState,
		}

		for _, metric := range metrics {
			if err := b.registerMetric(metric); err != nil {
				return fmt.Errorf("failed to register bundle metric: %w", err)
			}
		}
	}

	return nil
}

// registerMetric safely registers a Prometheus metric with proper error handling.
func (b *Bundle) registerMetric(collector prometheus.Collector) error {
	err := b.registry.Register(collector)
	if err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError
		if stderrors.As(err, &alreadyRegisteredErr) {
			// Metric already exists - this is not an error in most cases
			return nil
		}
		return fmt.Errorf("failed to register metric: %w", err)
	}
	return nil
}

// RecordHTTPRequest records metrics for an HTTP request.
func (b *Bundle) RecordHTTPRequest(method, endpoint string, statusCode int, duration time.Duration) {
	if b.httpRequestsTotal != nil {
		b.httpRequestsTotal.WithLabelValues(method, endpoint, strconv.Itoa(statusCode)).Inc()
	}
	if b.httpRequestDuration != nil {
		b.httpRequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
	}
}

// RecordGRPCRequest records metrics for a gRPC request.
func (b *Bundle) RecordGRPCRequest(method, statusCode string, duration time.Duration) {
	if b.grpcRequestsTotal != nil {
		b.grpcRequestsTotal.WithLabelValues(method, statusCode).Inc()
	}
	if b.grpcRequestDuration != nil {
		b.grpcRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
	}
}

// RecordHealthCheck records metrics for a health check.
func (b *Bundle) RecordHealthCheck(checkName, checkType string, success bool, duration time.Duration) {
	if b.healthCheckTotal != nil {
		status := "success"
		if !success {
			status = "failure"
		}
		b.healthCheckTotal.WithLabelValues(checkName, checkType, status).Inc()
	}
	if b.healthCheckDuration != nil {
		b.healthCheckDuration.WithLabelValues(checkName, checkType).Observe(duration.Seconds())
	}
}

// UpdateDatabaseConnections updates database connection pool metrics.
func (b *Bundle) UpdateDatabaseConnections(active, idle int) {
	if b.dbConnectionsActive != nil {
		b.dbConnectionsActive.Set(float64(active))
	}
	if b.dbConnectionsIdle != nil {
		b.dbConnectionsIdle.Set(float64(idle))
	}
}

// UpdateRedisConnections updates Redis connection pool metrics.
func (b *Bundle) UpdateRedisConnections(active, idle int) {
	if b.redisConnectionsActive != nil {
		b.redisConnectionsActive.Set(float64(active))
	}
	if b.redisConnectionsIdle != nil {
		b.redisConnectionsIdle.Set(float64(idle))
	}
}

// RecordJWTValidation records JWT token validation metrics.
func (b *Bundle) RecordJWTValidation(success bool, service string) {
	if b.jwtTokensValidated != nil {
		status := "success"
		if !success {
			status = "failure"
		}
		b.jwtTokensValidated.WithLabelValues(status, service).Inc()
	}
}

// UpdateCircuitBreakerState updates circuit breaker state metrics.
func (b *Bundle) UpdateCircuitBreakerState(name string, state int) {
	if b.circuitBreakerState != nil {
		b.circuitBreakerState.WithLabelValues(name).Set(float64(state))
	}
}

// CreateCustomCounter creates a new counter metric with consistent labeling and validation.
func (b *Bundle) CreateCustomCounter(name, help string, labelNames []string) (*prometheus.CounterVec, error) {
	if err := b.validateMetricDefinition(name, help, labelNames); err != nil {
		return nil, fmt.Errorf("invalid metric definition: %w", err)
	}

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   b.config.Namespace,
			Subsystem:   b.config.Subsystem,
			Name:        name,
			Help:        help,
			ConstLabels: b.config.ServiceLabels,
		},
		labelNames,
	)

	if err := b.registerMetric(counter); err != nil {
		return nil, fmt.Errorf("failed to register counter metric %s: %w", name, err)
	}

	return counter, nil
}

// CreateCustomGauge creates a new gauge metric with consistent labeling and validation.
func (b *Bundle) CreateCustomGauge(name, help string, labelNames []string) (*prometheus.GaugeVec, error) {
	if err := b.validateMetricDefinition(name, help, labelNames); err != nil {
		return nil, fmt.Errorf("invalid metric definition: %w", err)
	}

	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   b.config.Namespace,
			Subsystem:   b.config.Subsystem,
			Name:        name,
			Help:        help,
			ConstLabels: b.config.ServiceLabels,
		},
		labelNames,
	)

	if err := b.registerMetric(gauge); err != nil {
		return nil, fmt.Errorf("failed to register gauge metric %s: %w", name, err)
	}

	return gauge, nil
}

// CreateCustomHistogram creates a new histogram metric with consistent labeling and validation.
func (b *Bundle) CreateCustomHistogram(name, help string, labelNames []string) (*prometheus.HistogramVec, error) {
	if err := b.validateMetricDefinition(name, help, labelNames); err != nil {
		return nil, fmt.Errorf("invalid metric definition: %w", err)
	}

	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   b.config.Namespace,
			Subsystem:   b.config.Subsystem,
			Name:        name,
			Help:        help,
			Buckets:     b.config.HistogramBuckets,
			ConstLabels: b.config.ServiceLabels,
		},
		labelNames,
	)

	if err := b.registerMetric(histogram); err != nil {
		return nil, fmt.Errorf("failed to register histogram metric %s: %w", name, err)
	}

	return histogram, nil
}

// CreateCustomSummary creates a new summary metric with consistent labeling and validation.
func (b *Bundle) CreateCustomSummary(name, help string, labelNames []string, objectives map[float64]float64) (*prometheus.SummaryVec, error) {
	if err := b.validateMetricDefinition(name, help, labelNames); err != nil {
		return nil, fmt.Errorf("invalid metric definition: %w", err)
	}

	summary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:   b.config.Namespace,
			Subsystem:   b.config.Subsystem,
			Name:        name,
			Help:        help,
			Objectives:  objectives,
			ConstLabels: b.config.ServiceLabels,
		},
		labelNames,
	)

	if err := b.registerMetric(summary); err != nil {
		return nil, fmt.Errorf("failed to register summary metric %s: %w", name, err)
	}

	return summary, nil
}

// validateMetricDefinition validates metric names and labels to prevent security and performance issues.
func (b *Bundle) validateMetricDefinition(name, help string, labelNames []string) error {
	if !isValidMetricName(name) {
		return fmt.Errorf("invalid metric name format: %s", name)
	}

	if help == "" {
		return stderrors.New("metric help text is required")
	}

	// Prevent high cardinality
	if len(labelNames) > 15 {
		return fmt.Errorf("too many labels (%d), maximum 15 allowed to prevent high cardinality", len(labelNames))
	}

	// Validate label names
	for _, labelName := range labelNames {
		if !isValidLabelName(labelName) {
			return fmt.Errorf("invalid label name format: %s", labelName)
		}
	}

	return nil
}

// isValidLabelName validates Prometheus label naming conventions.
func isValidLabelName(name string) bool {
	if len(name) == 0 {
		return false
	}

	// First character must be letter or underscore
	first := name[0]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') && first != '_' {
		return false
	}

	// Remaining characters must be letters, digits, or underscores
	for i := 1; i < len(name); i++ {
		char := name[i]
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '_' {
			return false
		}
	}

	return true
}

// SecurityConfig contains security configuration for the metrics endpoint.
type SecurityConfig struct {
	MaxRequestsInFlight int           // Maximum concurrent requests (default: 3)
	Timeout             time.Duration // Request timeout (default: 10s)
	EnableBasicAuth     bool          // Enable basic authentication
	Username            string        // Basic auth username
	Password            string        // Basic auth password
}

// DefaultSecurityConfig returns secure defaults for metrics endpoint.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		MaxRequestsInFlight: 3,
		Timeout:             10 * time.Second,
		EnableBasicAuth:     false,
	}
}

// GetMetricsHandler returns an HTTP handler for the /metrics endpoint with basic security.
func (b *Bundle) GetMetricsHandler() http.Handler {
	return b.GetSecureMetricsHandler(DefaultSecurityConfig())
}

// GetSecureMetricsHandler returns a secured HTTP handler for the /metrics endpoint.
func (b *Bundle) GetSecureMetricsHandler(secConfig SecurityConfig) http.Handler {
	handler := promhttp.HandlerFor(
		b.gatherer,
		promhttp.HandlerOpts{
			EnableOpenMetrics:   true,
			Timeout:             secConfig.Timeout,
			MaxRequestsInFlight: secConfig.MaxRequestsInFlight,
		},
	)

	// Add basic authentication if configured
	if secConfig.EnableBasicAuth && secConfig.Username != "" && secConfig.Password != "" {
		return b.basicAuthMiddleware(handler, secConfig.Username, secConfig.Password)
	}

	return handler
}

// basicAuthMiddleware adds basic authentication to the metrics endpoint.
func (b *Bundle) basicAuthMiddleware(next http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Metrics"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CollectConnectionMetrics collects connection pool metrics from other bundles.
func (b *Bundle) CollectConnectionMetrics(ctx context.Context, app *framework.App) {
	if !b.config.EnableBundleMetrics {
		return
	}

	// This would be called periodically to collect metrics from other bundles
	// Implementation would depend on how other bundles expose their metrics
	// For now, this is a placeholder for the integration pattern
}

// HealthChecks returns health checks for the Prometheus metrics system.
func (b *Bundle) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		&PrometheusHealthCheck{
			bundle: b,
		},
	}
}

// PrometheusHealthCheck implements health checking for Prometheus metrics.
type PrometheusHealthCheck struct {
	bundle *Bundle
}

// Name returns the health check name.
func (c *PrometheusHealthCheck) Name() string {
	return "prometheus"
}

// Liveness performs a basic metrics system check.
func (c *PrometheusHealthCheck) Liveness(ctx context.Context) error {
	// Check that metrics can be gathered
	_, err := c.bundle.gatherer.Gather()
	if err != nil {
		return fmt.Errorf("failed to gather Prometheus metrics: %w", err)
	}
	return nil
}

// Readiness performs a comprehensive metrics system check.
func (c *PrometheusHealthCheck) Readiness(ctx context.Context) error {
	// Check basic functionality
	if err := c.Liveness(ctx); err != nil {
		return err
	}

	// Verify key metrics are registered and collecting data
	metricFamilies, err := c.bundle.gatherer.Gather()
	if err != nil {
		return fmt.Errorf("failed to gather metrics for readiness check: %w", err)
	}

	// Ensure we have at least some metrics registered
	if len(metricFamilies) == 0 {
		return stderrors.New("no metrics registered in Prometheus registry")
	}

	// Check for expected namespace metrics
	hasNamespaceMetrics := false
	for _, family := range metricFamilies {
		if family.Name != nil {
			name := *family.Name
			if strings.HasPrefix(name, c.bundle.config.Namespace) ||
				strings.HasPrefix(name, "go_") ||
				strings.HasPrefix(name, "process_") {
				hasNamespaceMetrics = true
				break
			}
		}
	}

	if !hasNamespaceMetrics {
		return fmt.Errorf("no metrics found for namespace %s", c.bundle.config.Namespace)
	}

	return nil
}

// isValidMetricName validates Prometheus metric naming conventions.
func isValidMetricName(name string) bool {
	if len(name) == 0 {
		return false
	}

	// First character must be letter or underscore
	first := name[0]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') && first != '_' && first != ':' {
		return false
	}

	// Remaining characters must be letters, digits, underscores, or colons
	for i := 1; i < len(name); i++ {
		char := name[i]
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '_' && char != ':' {
			return false
		}
	}

	return true
}

// MetricsCollector provides an interface for components to expose metrics.
type MetricsCollector interface {
	// CollectMetrics should update Prometheus metrics with current state.
	CollectMetrics(ctx context.Context, bundle *Bundle) error
}

// StartMetricsCollection starts periodic collection of metrics from registered collectors.
func (b *Bundle) StartMetricsCollection(ctx context.Context, collectors []MetricsCollector, interval time.Duration) {
	if len(collectors) == 0 || interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for _, collector := range collectors {
					if err := collector.CollectMetrics(ctx, b); err != nil {
						b.logger.Error().Err(err).Msg("Metrics collection error")
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop implements the Bundle interface for graceful shutdown.
// Prometheus bundle has no persistent resources requiring cleanup.
func (b *Bundle) Stop(ctx context.Context) error {
	// Metrics remain in Prometheus registry until process exit
	// No cleanup needed
	return nil
}
