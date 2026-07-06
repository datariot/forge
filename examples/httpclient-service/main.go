// Package main demonstrates a Forge microservice with HTTP client integration.
//
// This example shows how to:
//   - Use the HTTP client bundle for service-to-service communication
//   - Implement circuit breaker patterns for resilience
//   - Handle retries with exponential backoff
//   - Integrate JWT authentication with HTTP calls
//   - Use request/response logging and metrics
//   - Handle HTTP client errors gracefully
//
// # Prerequisites
//
// 1. Target service running for HTTP calls (optional - will use httpbin.org for demo)
// 2. JWT configuration if testing authentication
//
// # Run the service
//
//	go run main.go
//
// # Test the service
//
//	# Test basic HTTP calls
//	curl http://localhost:8081/api/test/get
//	curl -X POST http://localhost:8081/api/test/post
//
//	# Test circuit breaker
//	curl http://localhost:8081/api/test/unreliable
//
//	# Test with authentication
//	curl -H "Authorization: Bearer <token>" http://localhost:8081/api/test/auth
//
//	# Check circuit breaker status
//	curl http://localhost:8081/api/circuit-breaker/status
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/sony/gobreaker"

	"github.com/datariot/forge/bundles/httpclient"
	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// ServiceConfig extends BaseConfig with HTTP client configuration.
type ServiceConfig struct {
	config.BaseConfig `yaml:",inline"`

	// HTTP client configuration
	TargetServiceURL string `yaml:"target_service_url" env:"TARGET_SERVICE_URL"`
	ClientTimeout    string `yaml:"client_timeout" env:"CLIENT_TIMEOUT"`
	MaxRetries       int    `yaml:"max_retries" env:"MAX_RETRIES"`
	EnableAuth       bool   `yaml:"enable_auth" env:"ENABLE_AUTH"`
}

// DefaultServiceConfig returns configuration with defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		BaseConfig:       config.DefaultBaseConfig(),
		TargetServiceURL: "https://httpbin.org", // Use httpbin.org for demo
		ClientTimeout:    "30s",
		MaxRetries:       3,
		EnableAuth:       false,
	}
}

// Validate validates the service configuration.
func (c *ServiceConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.TargetServiceURL == "" {
		return fmt.Errorf("target_service_url is required")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative")
	}

	return nil
}

// HTTPClientService demonstrates HTTP client functionality.
type HTTPClientService struct {
	config        *ServiceConfig
	httpBundle    *httpclient.Bundle
	clientTimeout time.Duration
}

// NewHTTPClientService creates a new HTTP client service.
func NewHTTPClientService(config *ServiceConfig, httpBundle *httpclient.Bundle) *HTTPClientService {
	timeout, err := time.ParseDuration(config.ClientTimeout)
	if err != nil {
		timeout = 30 * time.Second
	}

	return &HTTPClientService{
		config:        config,
		httpBundle:    httpBundle,
		clientTimeout: timeout,
	}
}

// Start initializes the HTTP client service.
func (s *HTTPClientService) Start(ctx context.Context) error {
	log.Printf("HTTPClientService started with target: %s", s.config.TargetServiceURL)
	return nil
}

// Stop gracefully shuts down the service.
func (s *HTTPClientService) Stop(ctx context.Context) error {
	log.Printf("HTTPClientService stopping...")
	return s.httpBundle.Close()
}

// HealthChecks implements the HealthContributor interface.
func (s *HTTPClientService) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		&HTTPClientHealthCheck{
			client:    s.httpBundle.Client(),
			targetURL: s.config.TargetServiceURL + "/status/200",
		},
	}
}

// setupHTTPEndpoints configures HTTP endpoints for testing client functionality.
func (s *HTTPClientService) setupHTTPEndpoints(mux *http.ServeMux) {
	// Test endpoints
	mux.HandleFunc("/api/test/get", s.handleTestGet)
	mux.HandleFunc("/api/test/post", s.handleTestPost)
	mux.HandleFunc("/api/test/unreliable", s.handleTestUnreliable)
	mux.HandleFunc("/api/test/auth", s.handleTestAuth)

	// Circuit breaker status
	mux.HandleFunc("/api/circuit-breaker/status", s.handleCircuitBreakerStatus)

	// Client statistics
	mux.HandleFunc("/api/client/stats", s.handleClientStats)
}

// User represents a user for testing JSON serialization.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// handleTestGet demonstrates GET requests with error handling.
func (s *HTTPClientService) handleTestGet(w http.ResponseWriter, r *http.Request) {
	client := s.httpBundle.Client()

	// Make GET request to httpbin.org
	var response map[string]interface{}
	err := client.Get(r.Context(), "/get?test=true", &response)
	if err != nil {
		http.Error(w, fmt.Sprintf("GET request failed: %v", err), http.StatusBadGateway)
		return
	}

	// Return the response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"method":   "GET",
		"target":   s.config.TargetServiceURL + "/get",
		"response": response,
	})
}

// handleTestPost demonstrates POST requests with request body.
func (s *HTTPClientService) handleTestPost(w http.ResponseWriter, r *http.Request) {
	client := s.httpBundle.Client()

	// Create test user data
	user := User{
		ID:    "123",
		Name:  "Test User",
		Email: "test@example.com",
	}

	// Make POST request
	var response map[string]interface{}
	err := client.Post(r.Context(), "/post", user, &response)
	if err != nil {
		http.Error(w, fmt.Sprintf("POST request failed: %v", err), http.StatusBadGateway)
		return
	}

	// Return the response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"method":   "POST",
		"target":   s.config.TargetServiceURL + "/post",
		"sent":     user,
		"response": response,
	})
}

// handleTestUnreliable demonstrates circuit breaker functionality.
func (s *HTTPClientService) handleTestUnreliable(w http.ResponseWriter, r *http.Request) {
	client := s.httpBundle.Client()

	// Simulate unreliable service by randomly using error status codes
	statusCodes := []int{200, 500, 502, 503, 504}
	randomStatus := statusCodes[rand.Intn(len(statusCodes))]

	var response map[string]interface{}
	err := client.Get(r.Context(), "/status/"+strconv.Itoa(randomStatus), &response)

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		// Check if circuit breaker is open
		if err == httpclient.ErrCircuitBreakerOpen {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":         false,
				"error":           "Circuit breaker is open",
				"circuit_breaker": "OPEN",
				"message":         "Service is temporarily unavailable due to failures",
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
			"target":  s.config.TargetServiceURL + "/status/" + strconv.Itoa(randomStatus),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"method":      "GET",
		"target":      s.config.TargetServiceURL + "/status/" + strconv.Itoa(randomStatus),
		"status_code": randomStatus,
		"response":    response,
	})
}

// handleTestAuth demonstrates authenticated HTTP requests.
func (s *HTTPClientService) handleTestAuth(w http.ResponseWriter, r *http.Request) {
	client := s.httpBundle.Client()

	// Extract JWT token from Authorization header and add to context
	authHeader := r.Header.Get("Authorization")
	var ctx context.Context = r.Context()

	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]
		ctx = httpclient.WithJWTToken(ctx, token)
	}

	// Make authenticated request
	var response map[string]interface{}
	err := client.Get(ctx, "/bearer", &response)
	if err != nil {
		http.Error(w, fmt.Sprintf("Authenticated request failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"method":        "GET",
		"target":        s.config.TargetServiceURL + "/bearer",
		"authenticated": authHeader != "",
		"response":      response,
	})
}

// handleCircuitBreakerStatus shows circuit breaker status and metrics.
func (s *HTTPClientService) handleCircuitBreakerStatus(w http.ResponseWriter, r *http.Request) {
	client := s.httpBundle.Client()

	state := client.GetCircuitBreakerState()
	counts := client.GetCircuitBreakerCounts()

	status := map[string]interface{}{
		"state": state.String(),
		"counts": map[string]interface{}{
			"requests":              counts.Requests,
			"total_successes":       counts.TotalSuccesses,
			"total_failures":        counts.TotalFailures,
			"consecutive_successes": counts.ConsecutiveSuccesses,
			"consecutive_failures":  counts.ConsecutiveFailures,
		},
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleClientStats shows HTTP client connection statistics.
func (s *HTTPClientService) handleClientStats(w http.ResponseWriter, r *http.Request) {
	// This would show connection pool stats, request metrics, etc.
	// For now, return basic information
	stats := map[string]interface{}{
		"service":      s.config.ServiceName,
		"target_url":   s.config.TargetServiceURL,
		"timeout":      s.clientTimeout.String(),
		"max_retries":  s.config.MaxRetries,
		"auth_enabled": s.config.EnableAuth,
		"timestamp":    time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HTTPClientHealthCheck provides health checking for HTTP client.
type HTTPClientHealthCheck struct {
	client    *httpclient.Client
	targetURL string
}

// Name returns the health check name.
func (c *HTTPClientHealthCheck) Name() string {
	return "http-client"
}

// Liveness performs a basic HTTP client connectivity check.
func (c *HTTPClientHealthCheck) Liveness(ctx context.Context) error {
	return c.client.HealthCheck(ctx, c.targetURL)
}

// Readiness performs a comprehensive HTTP client readiness check.
func (c *HTTPClientHealthCheck) Readiness(ctx context.Context) error {
	// Check circuit breaker state
	state := c.client.GetCircuitBreakerState()
	if state == gobreaker.StateOpen {
		return fmt.Errorf("HTTP client circuit breaker is open")
	}

	// Check basic connectivity
	return c.client.HealthCheck(ctx, c.targetURL)
}

func main() {
	// Load configuration
	cfg := DefaultServiceConfig()
	cfg.ServiceName = "httpclient-service"

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Parse timeout
	timeout, err := time.ParseDuration(cfg.ClientTimeout)
	if err != nil {
		log.Fatalf("Invalid client timeout: %v", err)
	}

	// Create HTTP client bundle
	httpConfig := httpclient.Config{
		BaseURL: cfg.TargetServiceURL,
		Timeout: timeout,
		RetryConfig: httpclient.RetryConfig{
			MaxRetries:          cfg.MaxRetries,
			InitialInterval:     100 * time.Millisecond,
			MaxInterval:         5 * time.Second,
			Multiplier:          2.0,
			RandomizationFactor: 0.1,
		},
		CircuitBreakerConfig: httpclient.CircuitBreakerConfig{
			Name:        cfg.ServiceName + "-circuit-breaker",
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
		},
		EnableJWTAuth:        cfg.EnableAuth,
		EnableRequestLogging: true,
		EnableMetrics:        true,
		LogRequestBody:       true,
		LogResponseBody:      true,
		MaxLogBodySize:       1024,
		UserAgent:            fmt.Sprintf("%s/1.0", cfg.ServiceName),
	}

	httpBundle := httpclient.NewBundle(httpConfig)

	// Create HTTP client service
	clientService := NewHTTPClientService(&cfg, httpBundle)

	// Create the application with HTTP client integration
	app, err := framework.New(
		framework.WithConfig(&cfg.BaseConfig),
		framework.WithVersion("1.0.0"),
		framework.WithBundle(httpBundle),
		framework.WithComponent(clientService),
		framework.WithHealthContributor(clientService),
		framework.WithStartupHook(func(ctx context.Context, app *framework.App) error {
			log.Printf("HTTP client endpoints available:")
			log.Printf("  GET  /api/test/get - Test GET request")
			log.Printf("  POST /api/test/post - Test POST request")
			log.Printf("  GET  /api/test/unreliable - Test circuit breaker")
			log.Printf("  GET  /api/test/auth - Test authenticated request")
			log.Printf("  GET  /api/circuit-breaker/status - Circuit breaker status")
			log.Printf("  GET  /api/client/stats - Client statistics")
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	log.Printf("Starting %s with HTTP client integration...", cfg.ServiceName)
	log.Printf("Target service URL: %s", cfg.TargetServiceURL)
	log.Printf("Client timeout: %s", cfg.ClientTimeout)
	log.Printf("Max retries: %d", cfg.MaxRetries)

	// Run the application
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
