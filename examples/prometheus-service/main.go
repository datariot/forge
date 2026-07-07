// Package main demonstrates a Forge microservice with Prometheus metrics integration.
//
// This example shows how to:
//   - Use the Prometheus bundle for metrics collection
//   - Create and register custom metrics
//   - Integrate metrics with other bundles (PostgreSQL, Redis, JWT)
//   - Provide comprehensive observability and monitoring
//   - Use Grafana dashboards for visualization
//
// # Run the service
//
//	go run main.go
//
// # View metrics
//
//	# Prometheus metrics endpoint
//	curl http://localhost:8081/metrics
//
//	# Test endpoints to generate metrics
//	curl http://localhost:8081/api/users
//	curl -X POST http://localhost:8081/api/users -d '{"name":"John","email":"john@example.com"}'
//	curl http://localhost:8081/api/simulate/error
//	curl http://localhost:8081/api/simulate/slow
//
//	# Health checks (generates health metrics)
//	curl http://localhost:8081/health
//
// # Grafana Dashboard
//
//	Import the dashboard from bundles/prometheus/grafana-dashboard.json
//	Update the namespace variable to match your service name
package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	forgePrometheus "github.com/datariot/forge/bundles/prometheus"
	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// ServiceConfig extends BaseConfig with Prometheus-specific configuration.
type ServiceConfig struct {
	config.BaseConfig `yaml:",inline"`

	// Prometheus configuration
	MetricsNamespace string `yaml:"metrics_namespace" env:"METRICS_NAMESPACE"`
	MetricsSubsystem string `yaml:"metrics_subsystem" env:"METRICS_SUBSYSTEM"`
}

// DefaultServiceConfig returns configuration with defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		BaseConfig:       config.DefaultBaseConfig(),
		MetricsNamespace: "prometheus_example",
		MetricsSubsystem: "api",
	}
}

// Validate validates the service configuration.
func (c *ServiceConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.MetricsNamespace == "" {
		return fmt.Errorf("metrics_namespace is required")
	}

	return nil
}

// MetricsService demonstrates Prometheus metrics functionality.
type MetricsService struct {
	config           *ServiceConfig
	prometheusBundle *forgePrometheus.Bundle

	// Custom metrics
	userOperations    *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
	activeConnections prometheus.Gauge
	cacheHitRatio     *prometheus.GaugeVec
}

// NewMetricsService creates a new metrics service.
func NewMetricsService(config *ServiceConfig, prometheusBundle *forgePrometheus.Bundle) (*MetricsService, error) {
	service := &MetricsService{
		config:           config,
		prometheusBundle: prometheusBundle,
	}

	// Create custom metrics
	if err := service.initializeCustomMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize custom metrics: %w", err)
	}

	return service, nil
}

// Start initializes the metrics service.
func (s *MetricsService) Start(ctx context.Context) error {
	log.Printf("MetricsService started with Prometheus integration")

	// Start background metrics collection
	s.startMetricsCollection(ctx)

	return nil
}

// Stop gracefully shuts down the service.
func (s *MetricsService) Stop(ctx context.Context) error {
	log.Printf("MetricsService stopping...")
	return nil
}

// HealthChecks implements the HealthContributor interface.
func (s *MetricsService) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		&MetricsServiceHealthCheck{
			prometheusBundle: s.prometheusBundle,
		},
	}
}

// initializeCustomMetrics creates custom application metrics.
func (s *MetricsService) initializeCustomMetrics() error {
	var err error

	// User operation metrics
	s.userOperations, err = s.prometheusBundle.CreateCustomCounter(
		"user_operations_total",
		"Total number of user operations performed",
		[]string{"operation", "status"},
	)
	if err != nil {
		return fmt.Errorf("failed to create user operations counter: %w", err)
	}

	// Request duration metrics with custom buckets
	s.requestDuration, err = s.prometheusBundle.CreateCustomHistogram(
		"request_duration_seconds",
		"Duration of requests in seconds",
		[]string{"endpoint", "method"},
	)
	if err != nil {
		return fmt.Errorf("failed to create request duration histogram: %w", err)
	}

	// Active connections gauge
	s.activeConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: s.config.MetricsNamespace,
			Subsystem: s.config.MetricsSubsystem,
			Name:      "active_connections",
			Help:      "Number of active connections",
		},
	)
	if err := s.prometheusBundle.Registry().Register(s.activeConnections); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError
		if !stderrors.As(err, &alreadyRegisteredErr) {
			return fmt.Errorf("failed to register active connections gauge: %w", err)
		}
	}

	// Cache hit ratio
	s.cacheHitRatio, err = s.prometheusBundle.CreateCustomGauge(
		"cache_hit_ratio",
		"Cache hit ratio percentage",
		[]string{"cache_type"},
	)
	if err != nil {
		return fmt.Errorf("failed to create cache hit ratio gauge: %w", err)
	}

	return nil
}

// setupHTTPEndpoints configures HTTP endpoints for metrics demonstration.
func (s *MetricsService) setupHTTPEndpoints(mux *http.ServeMux) {
	// API endpoints that generate metrics
	mux.HandleFunc("/api/users", s.handleUsers)
	mux.HandleFunc("/api/simulate/error", s.handleSimulateError)
	mux.HandleFunc("/api/simulate/slow", s.handleSimulateSlow)

	// Metrics information endpoint
	mux.HandleFunc("/api/metrics/info", s.handleMetricsInfo)

	// Custom metrics update endpoint
	mux.HandleFunc("/api/metrics/update", s.handleUpdateMetrics)
}

// User represents a user for demonstration.
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// handleUsers handles user operations and records metrics.
func (s *MetricsService) handleUsers(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	endpoint := "/api/users"

	// Record request metrics
	defer func() {
		duration := time.Since(start)
		s.requestDuration.WithLabelValues(endpoint, r.Method).Observe(duration.Seconds())
		s.prometheusBundle.RecordHTTPRequest(r.Method, endpoint, 200, duration)
	}()

	switch r.Method {
	case http.MethodGet:
		s.handleGetUsers(w, r)
	case http.MethodPost:
		s.handleCreateUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetUsers simulates getting users and records metrics.
func (s *MetricsService) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	// Simulate cache hit/miss
	cacheHit := rand.Float64() > 0.3 // 70% cache hit rate

	if cacheHit {
		s.cacheHitRatio.WithLabelValues("users").Set(70.0)
	} else {
		s.cacheHitRatio.WithLabelValues("users").Set(30.0)
	}

	// Record operation metrics
	s.userOperations.WithLabelValues("get", "success").Inc()

	// Simulate user data
	users := []User{
		{
			ID:        "1",
			Name:      "Alice Smith",
			Email:     "alice@example.com",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			ID:        "2",
			Name:      "Bob Johnson",
			Email:     "bob@example.com",
			CreatedAt: time.Now().Add(-12 * time.Hour),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"users":     users,
		"cache_hit": cacheHit,
		"timestamp": time.Now().UTC(),
	})
}

// handleCreateUser simulates creating a user and records metrics.
func (s *MetricsService) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		s.userOperations.WithLabelValues("create", "error").Inc()
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Simulate user creation
	user.ID = fmt.Sprintf("%d", time.Now().Unix())
	user.CreatedAt = time.Now().UTC()

	// Record successful operation
	s.userOperations.WithLabelValues("create", "success").Inc()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"user":      user,
		"created":   true,
		"timestamp": time.Now().UTC(),
	})
}

// handleSimulateError simulates errors for metrics demonstration.
func (s *MetricsService) handleSimulateError(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	endpoint := "/api/simulate/error"

	// Randomly simulate different error types
	errorTypes := []int{400, 404, 500, 502, 503}
	statusCode := errorTypes[rand.Intn(len(errorTypes))]

	// Record metrics
	duration := time.Since(start)
	s.requestDuration.WithLabelValues(endpoint, r.Method).Observe(duration.Seconds())
	s.prometheusBundle.RecordHTTPRequest(r.Method, endpoint, statusCode, duration)

	// Record error metrics
	s.userOperations.WithLabelValues("simulate", "error").Inc()

	http.Error(w, fmt.Sprintf("Simulated error: %d", statusCode), statusCode)
}

// handleSimulateSlow simulates slow requests for latency metrics.
func (s *MetricsService) handleSimulateSlow(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	endpoint := "/api/simulate/slow"

	// Simulate slow operation (1-5 seconds)
	delay := time.Duration(rand.Intn(4)+1) * time.Second
	time.Sleep(delay)

	// Record metrics
	duration := time.Since(start)
	s.requestDuration.WithLabelValues(endpoint, r.Method).Observe(duration.Seconds())
	s.prometheusBundle.RecordHTTPRequest(r.Method, endpoint, 200, duration)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Slow operation completed",
		"delay":     delay.String(),
		"duration":  duration.String(),
		"timestamp": time.Now().UTC(),
	})
}

// handleMetricsInfo provides information about available metrics.
func (s *MetricsService) handleMetricsInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"service":          s.config.ServiceName,
		"namespace":        s.config.MetricsNamespace,
		"subsystem":        s.config.MetricsSubsystem,
		"metrics_endpoint": "/metrics",
		"available_metrics": []string{
			"http_requests_total",
			"http_request_duration_seconds",
			"grpc_requests_total",
			"grpc_request_duration_seconds",
			"health_checks_total",
			"health_check_duration_seconds",
			"user_operations_total",
			"request_duration_seconds",
			"active_connections",
			"cache_hit_ratio",
		},
		"custom_metrics": []string{
			"user_operations_total",
			"request_duration_seconds",
			"active_connections",
			"cache_hit_ratio",
		},
		"grafana_dashboard": "/bundles/prometheus/grafana-dashboard.json",
		"timestamp":         time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleUpdateMetrics allows manual metrics updates for demonstration.
func (s *MetricsService) handleUpdateMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Update various metrics for demonstration
	s.activeConnections.Set(float64(rand.Intn(50) + 10))
	s.cacheHitRatio.WithLabelValues("users").Set(float64(rand.Intn(30) + 70))
	s.cacheHitRatio.WithLabelValues("sessions").Set(float64(rand.Intn(20) + 80))

	// Simulate database connections
	s.prometheusBundle.UpdateDatabaseConnections(rand.Intn(10)+5, rand.Intn(5)+2)

	// Simulate Redis connections
	s.prometheusBundle.UpdateRedisConnections(rand.Intn(8)+3, rand.Intn(3)+1)

	// Record JWT validation metrics
	s.prometheusBundle.RecordJWTValidation(true, "test-service")
	if rand.Float64() > 0.9 { // 10% failure rate
		s.prometheusBundle.RecordJWTValidation(false, "test-service")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"updated":   true,
		"message":   "Metrics updated with random values",
		"timestamp": time.Now().UTC(),
	})
}

// startMetricsCollection starts background metrics collection.
func (s *MetricsService) startMetricsCollection(ctx context.Context) {
	// Update connection metrics periodically
	ticker := time.NewTicker(15 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Simulate changing connection counts
				s.activeConnections.Set(float64(rand.Intn(20) + 10))

				// Update cache hit ratios
				s.cacheHitRatio.WithLabelValues("users").Set(float64(rand.Intn(20) + 75))
				s.cacheHitRatio.WithLabelValues("sessions").Set(float64(rand.Intn(15) + 85))

			case <-ctx.Done():
				return
			}
		}
	}()
}

// MetricsServiceHealthCheck provides service-specific health checking with metrics.
type MetricsServiceHealthCheck struct {
	prometheusBundle *forgePrometheus.Bundle
}

// Name returns the health check name.
func (c *MetricsServiceHealthCheck) Name() string {
	return "metrics-service"
}

// Liveness performs a basic service health check.
func (c *MetricsServiceHealthCheck) Liveness(ctx context.Context) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.prometheusBundle.RecordHealthCheck("metrics-service", "liveness", true, duration)
	}()

	// Simple liveness check
	return nil
}

// Readiness performs a comprehensive service readiness check.
func (c *MetricsServiceHealthCheck) Readiness(ctx context.Context) error {
	start := time.Now()
	success := true

	defer func() {
		duration := time.Since(start)
		c.prometheusBundle.RecordHealthCheck("metrics-service", "readiness", success, duration)
	}()

	// Check that Prometheus metrics can be gathered
	_, err := c.prometheusBundle.Gatherer().Gather()
	if err != nil {
		success = false
		return fmt.Errorf("failed to gather Prometheus metrics: %w", err)
	}

	return nil
}

func main() {
	// Load configuration
	cfg := DefaultServiceConfig()
	cfg.ServiceName = "prometheus-service"

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Create Prometheus bundle
	prometheusConfig := forgePrometheus.Config{
		Namespace:            cfg.MetricsNamespace,
		Subsystem:            cfg.MetricsSubsystem,
		EnableDefaultMetrics: true,
		EnableHTTPMetrics:    true,
		EnableGRPCMetrics:    true,
		EnableHealthMetrics:  true,
		EnableBundleMetrics:  true,
		ServiceLabels: map[string]string{
			"service": cfg.ServiceName,
			"version": "1.0.0",
			"env":     cfg.AppEnv,
		},
	}

	prometheusBundle := forgePrometheus.NewBundle(prometheusConfig)

	// Create metrics service
	metricsService, err := NewMetricsService(&cfg, prometheusBundle)
	if err != nil {
		log.Fatalf("Failed to create metrics service: %v", err)
	}

	// Create the application with Prometheus integration
	app, err := framework.New(
		framework.WithConfig(&cfg.BaseConfig),
		framework.WithVersion("1.0.0"),
		framework.WithBundle(prometheusBundle),
		framework.WithComponent(metricsService),
		framework.WithHealthContributor(metricsService),
		framework.WithStartupHook(func(ctx context.Context, app *framework.App) error {
			log.Printf("Prometheus metrics endpoints available:")
			log.Printf("  GET  /metrics - Prometheus metrics endpoint")
			log.Printf("  GET  /api/users - Get users (generates metrics)")
			log.Printf("  POST /api/users - Create user (generates metrics)")
			log.Printf("  GET  /api/simulate/error - Simulate errors")
			log.Printf("  GET  /api/simulate/slow - Simulate slow requests")
			log.Printf("  GET  /api/metrics/info - Metrics information")
			log.Printf("  POST /api/metrics/update - Update test metrics")
			log.Printf("")
			log.Printf("Grafana Dashboard: bundles/prometheus/grafana-dashboard.json")
			log.Printf("Set namespace variable to: %s", cfg.MetricsNamespace)
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	log.Printf("Starting %s with Prometheus metrics...", cfg.ServiceName)
	log.Printf("Metrics namespace: %s", cfg.MetricsNamespace)
	log.Printf("Metrics subsystem: %s", cfg.MetricsSubsystem)

	// Run the application
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
