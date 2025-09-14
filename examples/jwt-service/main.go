// Package main demonstrates a Forge microservice with JWT authentication.
//
// This example shows how to:
//   - Configure JWT authentication for service-to-service communication
//   - Implement authenticated gRPC services
//   - Protect HTTP endpoints with JWT middleware
//   - Use service identity and permissions
//   - Handle authentication errors properly
//
// # Prerequisites
//
// 1. Generate a strong JWT secret key (32+ bytes)
// 2. Configure environment variables for authentication
//
// # Run the service
//
//   JWT_SECRET="your-very-secure-secret-key-here-32-bytes-minimum" \
//   JWT_ISSUER="auth-service" \
//   JWT_AUDIENCE="forge-services" \
//   go run main.go
//
// # Test Authentication
//
//   # Generate a token (in a real setup, your auth service would do this)
//   curl -X POST http://localhost:8081/auth/token \
//     -H "Content-Type: application/json" \
//     -d '{"service_id": "test-client", "permissions": ["read:users"]}'
//
//   # Use token to access protected endpoint
//   curl -H "Authorization: Bearer <token>" http://localhost:8081/api/protected
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/datariot/forge/bundles/jwt"
	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// ServiceConfig extends BaseConfig with JWT-specific configuration.
type ServiceConfig struct {
	config.BaseConfig `yaml:",inline"`

	// JWT configuration
	JWTSecret   string `yaml:"jwt_secret" env:"JWT_SECRET"`
	JWTIssuer   string `yaml:"jwt_issuer" env:"JWT_ISSUER"`
	JWTAudience string `yaml:"jwt_audience" env:"JWT_AUDIENCE"`
}

// DefaultServiceConfig returns configuration with defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		BaseConfig:  config.DefaultBaseConfig(),
		JWTIssuer:   "forge-auth",
		JWTAudience: "forge-services",
	}
}

// Validate validates the service configuration.
func (c *ServiceConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET environment variable is required")
	}

	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters for security")
	}

	if c.JWTIssuer == "" {
		return fmt.Errorf("JWT_ISSUER is required")
	}

	if c.JWTAudience == "" {
		return fmt.Errorf("JWT_AUDIENCE is required")
	}

	return nil
}

// AuthenticatedService demonstrates a service component with JWT authentication.
type AuthenticatedService struct {
	config    *ServiceConfig
	jwtBundle *jwt.Bundle
}

// NewAuthenticatedService creates a new authenticated service.
func NewAuthenticatedService(config *ServiceConfig, jwtBundle *jwt.Bundle) *AuthenticatedService {
	return &AuthenticatedService{
		config:    config,
		jwtBundle: jwtBundle,
	}
}

// Start initializes the authenticated service.
func (s *AuthenticatedService) Start(ctx context.Context) error {
	log.Printf("AuthenticatedService started with JWT authentication")
	return nil
}

// Stop gracefully shuts down the service.
func (s *AuthenticatedService) Stop(ctx context.Context) error {
	log.Printf("AuthenticatedService stopping...")
	return nil
}

// HealthChecks implements the HealthContributor interface.
func (s *AuthenticatedService) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		forgeHealth.NewAlwaysHealthyCheck("jwt-service"),
	}
}

// setupHTTPEndpoints configures HTTP endpoints with JWT authentication.
func (s *AuthenticatedService) setupHTTPEndpoints(mux *http.ServeMux, jwtBundle *jwt.Bundle) {
	// Public endpoint (no authentication required)
	mux.HandleFunc("/api/public", s.handlePublic)

	// Protected endpoint (requires authentication)
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/api/protected", s.handleProtected)
	protectedMux.HandleFunc("/api/admin", s.handleAdmin)

	// Apply JWT middleware to protected endpoints
	mux.Handle("/api/protected", jwtBundle.HTTPMiddleware(http.HandlerFunc(s.handleProtected)))
	mux.Handle("/api/admin", jwtBundle.RequirePermissions("admin")(http.HandlerFunc(s.handleAdmin)))

	// Token generation endpoint (for testing - in production this would be in auth service)
	mux.HandleFunc("/auth/token", s.handleGenerateToken)
}

// handlePublic handles public endpoints that don't require authentication.
func (s *AuthenticatedService) handlePublic(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"message": "This is a public endpoint",
		"service": s.config.ServiceName,
		"time":    time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleProtected handles protected endpoints that require authentication.
func (s *AuthenticatedService) handleProtected(w http.ResponseWriter, r *http.Request) {
	// Extract service claims from context
	claims := jwt.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	response := map[string]interface{}{
		"message":      "This is a protected endpoint",
		"service":      s.config.ServiceName,
		"authenticated_service": claims.ServiceName,
		"service_id":   claims.ServiceID,
		"permissions":  claims.Permissions,
		"time":         time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAdmin handles admin endpoints that require specific permissions.
func (s *AuthenticatedService) handleAdmin(w http.ResponseWriter, r *http.Request) {
	claims := jwt.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	response := map[string]interface{}{
		"message":      "This is an admin endpoint",
		"service":      s.config.ServiceName,
		"authenticated_service": claims.ServiceName,
		"service_id":   claims.ServiceID,
		"permissions":  claims.Permissions,
		"admin_access": jwt.HasPermission(r.Context(), "admin"),
		"time":         time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGenerateToken generates JWT tokens for testing (demo purposes only).
func (s *AuthenticatedService) handleGenerateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		ServiceID   string   `json:"service_id"`
		ServiceName string   `json:"service_name"`
		Permissions []string `json:"permissions"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if request.ServiceID == "" {
		request.ServiceID = request.ServiceName
	}
	if request.ServiceName == "" {
		request.ServiceName = "test-service"
	}

	// Generate token
	token, err := s.jwtBundle.GenerateToken(request.ServiceID, request.ServiceName, request.Permissions)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate token: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"token":      token,
		"service_id": request.ServiceID,
		"expires_in": "24h",
		"type":       "Bearer",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	// Load configuration
	cfg := DefaultServiceConfig()
	cfg.ServiceName = "jwt-service"

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Create JWT bundle
	jwtConfig := jwt.Config{
		SecretKey:     []byte(cfg.JWTSecret),
		Issuer:        cfg.JWTIssuer,
		Audience:      cfg.JWTAudience,
		TokenDuration: 24 * time.Hour,
		ServiceName:   cfg.ServiceName,
		ClockSkew:     5 * time.Minute,
		RequireHTTPS:  cfg.IsProduction(),
		SkipPaths:     []string{"/health", "/metrics", "/auth/token", "/api/public"},
	}

	jwtBundle := jwt.NewBundle(jwtConfig)

	// Create authenticated service
	authService := NewAuthenticatedService(&cfg, jwtBundle)

	// Create the application with JWT authentication
	app, err := framework.New(
		framework.WithConfig(&cfg.BaseConfig),
		framework.WithVersion("1.0.0"),
		framework.WithBundle(jwtBundle),
		framework.WithComponent(authService),
		framework.WithHealthContributor(authService),
		// Add JWT authentication interceptor for gRPC
		framework.WithUnaryInterceptor(jwtBundle.UnaryServerInterceptor()),
		// Setup HTTP endpoints with authentication
		framework.WithStartupHook(func(ctx context.Context, app *framework.App) error {
			// In a real application, you might set up authenticated HTTP routes here
			// For this example, we'll handle it in the HTTP server configuration
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	log.Printf("Starting %s with JWT authentication...", cfg.ServiceName)
	log.Printf("JWT Issuer: %s", cfg.JWTIssuer)
	log.Printf("JWT Audience: %s", cfg.JWTAudience)
	log.Printf("Protected endpoints require valid JWT tokens")

	// Run the application
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}