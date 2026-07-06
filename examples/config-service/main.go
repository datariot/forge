// Package main demonstrates a Forge microservice with automatic configuration loading.
//
// This example shows how to:
//   - Use the configuration loader bundle for automatic config management
//   - Load configuration from multiple sources (files, environment, defaults)
//   - Handle configuration validation and secure data
//   - Implement hot reload for configuration changes
//   - Debug configuration loading with detailed information
//
// # Configuration Sources (Priority Order)
//
// 1. Environment variables (highest priority)
// 2. Configuration files (./config.yaml, ./config.json)
// 3. Default values from struct tags (lowest priority)
//
// # Run the service
//
//	# With config file
//	echo 'service_name: "config-demo"
//	database_url: "postgres://localhost:5432/demo"
//	api_key: "secret-key-123"
//	debug: true' > config.yaml
//	go run main.go
//
//	# With environment variables (overrides file)
//	DATABASE_URL="postgres://prod:5432/proddb" \
//	API_KEY="prod-secret-456" \
//	DEBUG="false" \
//	go run main.go
//
//	# Hot reload testing
//	# Edit config.yaml while service is running to see hot reload
//
// # Test configuration endpoints
//
//	curl http://localhost:8081/api/config/info
//	curl http://localhost:8081/api/config/reload
//	curl http://localhost:8081/api/config/sources
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/datariot/forge/bundles/configloader"
	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// ServiceConfig extends BaseConfig with automatic configuration loading.
type ServiceConfig struct {
	config.BaseConfig `yaml:",inline" env:",inline"`

	// Service-specific configuration with tags for automatic loading
	DatabaseURL    string        `yaml:"database_url" env:"DATABASE_URL" validate:"required" sensitive:"true"`
	RedisURL       string        `yaml:"redis_url" env:"REDIS_URL" default:"redis://localhost:6379/0"`
	APIKey         string        `yaml:"api_key" env:"API_KEY" validate:"required" sensitive:"true"`
	JWTSecret      string        `yaml:"jwt_secret" env:"JWT_SECRET" validate:"required" sensitive:"true"`
	Debug          bool          `yaml:"debug" env:"DEBUG" default:"false"`
	MaxConnections int           `yaml:"max_connections" env:"MAX_CONNECTIONS" default:"10"`
	CacheTimeout   time.Duration `yaml:"cache_timeout" env:"CACHE_TIMEOUT" default:"1h"`
	Features       []string      `yaml:"features" env:"FEATURES" default:"auth,cache"`

	// Nested configuration
	ExternalServices ExternalServicesConfig `yaml:"external_services"`
}

// ExternalServicesConfig contains configuration for external service integration.
type ExternalServicesConfig struct {
	UserServiceURL  string `yaml:"user_service_url" env:"USER_SERVICE_URL" default:"http://localhost:8080"`
	EmailServiceURL string `yaml:"email_service_url" env:"EMAIL_SERVICE_URL" default:"http://localhost:8081"`
	Timeout         string `yaml:"timeout" env:"EXTERNAL_TIMEOUT" default:"30s"`
}

// DefaultServiceConfig returns configuration with defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		BaseConfig: config.DefaultBaseConfig(),
	}
}

// Validate validates the service configuration.
func (c *ServiceConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.DatabaseURL == "" {
		return fmt.Errorf("database_url is required")
	}

	if c.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}

	if len(c.APIKey) < 16 {
		return fmt.Errorf("api_key must be at least 16 characters")
	}

	if c.JWTSecret == "" {
		return fmt.Errorf("jwt_secret is required")
	}

	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("jwt_secret must be at least 32 characters")
	}

	if c.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be positive")
	}

	if c.CacheTimeout <= 0 {
		return fmt.Errorf("cache_timeout must be positive")
	}

	return nil
}

// ConfigService demonstrates configuration loading and management.
type ConfigService struct {
	config       *ServiceConfig
	configBundle *configloader.Bundle
	loadResult   *configloader.LoadResult
}

// NewConfigService creates a new configuration service.
func NewConfigService(config *ServiceConfig, configBundle *configloader.Bundle, loadResult *configloader.LoadResult) *ConfigService {
	return &ConfigService{
		config:       config,
		configBundle: configBundle,
		loadResult:   loadResult,
	}
}

// Start initializes the configuration service.
func (s *ConfigService) Start(ctx context.Context) error {
	log.Printf("ConfigService started with automatic configuration loading")

	// Setup configuration change monitoring
	s.configBundle.Loader().OnConfigChange(func(newConfig interface{}) {
		if cfg, ok := newConfig.(*ServiceConfig); ok {
			log.Printf("Configuration changed! New debug setting: %v", cfg.Debug)
			log.Printf("New max connections: %d", cfg.MaxConnections)
			// In a real application, you would handle configuration changes here
		}
	})

	// Start file watching if enabled
	if s.configBundle != nil {
		if err := s.configBundle.StartWatching(ctx, s.config); err != nil {
			log.Printf("Warning: Failed to start configuration file watching: %v", err)
		}
	}

	return nil
}

// Stop gracefully shuts down the service.
func (s *ConfigService) Stop(ctx context.Context) error {
	log.Printf("ConfigService stopping...")
	return s.configBundle.Close()
}

// HealthChecks implements the HealthContributor interface.
func (s *ConfigService) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		&ConfigServiceHealthCheck{
			config: s.config,
		},
	}
}

// setupHTTPEndpoints configures HTTP endpoints for configuration management.
func (s *ConfigService) setupHTTPEndpoints(mux *http.ServeMux) {
	// Configuration information endpoint
	mux.HandleFunc("/api/config/info", s.handleConfigInfo)

	// Configuration reload endpoint
	mux.HandleFunc("/api/config/reload", s.handleConfigReload)

	// Configuration sources endpoint
	mux.HandleFunc("/api/config/sources", s.handleConfigSources)

	// Configuration validation endpoint
	mux.HandleFunc("/api/config/validate", s.handleConfigValidate)
}

// handleConfigInfo returns current configuration information (sanitized).
func (s *ConfigService) handleConfigInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"service":         s.config.ServiceName,
		"environment":     s.config.AppEnv,
		"debug":           s.config.Debug,
		"max_connections": s.config.MaxConnections,
		"cache_timeout":   s.config.CacheTimeout.String(),
		"features":        s.config.Features,
		"external_services": map[string]interface{}{
			"user_service_url":  s.config.ExternalServices.UserServiceURL,
			"email_service_url": s.config.ExternalServices.EmailServiceURL,
			"timeout":           s.config.ExternalServices.Timeout,
		},
		// Sensitive fields are automatically redacted
		"database_url": "[REDACTED]",
		"api_key":      "[REDACTED]",
		"jwt_secret":   "[REDACTED]",
		"load_info":    s.configBundle.Loader().GetConfigInfo(),
		"timestamp":    time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleConfigReload manually reloads configuration.
func (s *ConfigService) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create new config instance
	newConfig := DefaultServiceConfig()

	// Reload configuration
	result, err := s.configBundle.Loader().Reload(&newConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Configuration reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Update current config (in a real app, you'd need more sophisticated handling)
	s.config = &newConfig

	response := map[string]interface{}{
		"reloaded":  true,
		"timestamp": time.Now().UTC(),
		"load_info": result,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleConfigSources returns information about configuration sources.
func (s *ConfigService) handleConfigSources(w http.ResponseWriter, r *http.Request) {
	sources := map[string]interface{}{
		"load_result": s.loadResult,
		"loader_info": s.configBundle.Loader().GetConfigInfo(),
		"timestamp":   time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sources)
}

// handleConfigValidate validates the current configuration.
func (s *ConfigService) handleConfigValidate(w http.ResponseWriter, r *http.Request) {
	err := s.config.Validate()

	response := map[string]interface{}{
		"valid":     err == nil,
		"timestamp": time.Now().UTC(),
	}

	if err != nil {
		response["error"] = err.Error()
		w.WriteHeader(http.StatusBadRequest)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ConfigServiceHealthCheck provides service-specific health checking.
type ConfigServiceHealthCheck struct {
	config *ServiceConfig
}

// Name returns the health check name.
func (c *ConfigServiceHealthCheck) Name() string {
	return "config-service"
}

// Liveness performs a basic service health check.
func (c *ConfigServiceHealthCheck) Liveness(ctx context.Context) error {
	// Check that configuration is valid
	return c.config.Validate()
}

// Readiness performs a comprehensive service readiness check.
func (c *ConfigServiceHealthCheck) Readiness(ctx context.Context) error {
	// Check configuration validity
	if err := c.config.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Check that required services are configured
	if c.config.DatabaseURL == "" {
		return fmt.Errorf("database URL not configured")
	}

	if c.config.APIKey == "" {
		return fmt.Errorf("API key not configured")
	}

	return nil
}

func main() {
	// Create configuration loader
	loaderConfig := configloader.Config{
		ConfigPaths: []string{
			"./config.yaml",
			"./config.yml",
			"./config.json",
			"./config/service.yaml",
		},
		EnvPrefix:         "FORGE_DEMO",
		WatchFiles:        true, // Enable hot reload
		RequireConfigFile: false,
		ValidateOnLoad:    true,
		SecureLogging:     true,
	}

	configBundle := configloader.NewBundle(loaderConfig)

	// Load initial configuration
	cfg := DefaultServiceConfig()
	cfg.ServiceName = "config-service"

	result, err := configBundle.Loader().Load(&cfg)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded successfully!")
	log.Printf("Loaded from: %s", result.LoadedFrom)
	log.Printf("Sources used: %v", result.Sources)
	log.Printf("Environment variables: %v", result.EnvVarsUsed)
	log.Printf("Defaults applied: %v", result.DefaultsApplied)

	// Create configuration service
	configService := NewConfigService(&cfg, configBundle, result)

	// Create the application with configuration loading
	app, err := framework.New(
		framework.WithConfig(&cfg.BaseConfig),
		framework.WithVersion("1.0.0"),
		framework.WithBundle(configBundle),
		framework.WithComponent(configService),
		framework.WithHealthContributor(configService),
		framework.WithStartupHook(func(ctx context.Context, app *framework.App) error {
			log.Printf("Configuration management endpoints available:")
			log.Printf("  GET  /api/config/info - Configuration information (sanitized)")
			log.Printf("  POST /api/config/reload - Reload configuration")
			log.Printf("  GET  /api/config/sources - Configuration source details")
			log.Printf("  GET  /api/config/validate - Validate current configuration")
			log.Printf("")
			log.Printf("Configuration Features:")
			log.Printf("  - Automatic environment variable binding")
			log.Printf("  - Configuration file loading (YAML/JSON)")
			log.Printf("  - Hot reload enabled: %v", loaderConfig.WatchFiles)
			log.Printf("  - Secure logging: %v", loaderConfig.SecureLogging)
			log.Printf("  - Validation on load: %v", loaderConfig.ValidateOnLoad)
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	log.Printf("Starting %s with automatic configuration loading...", cfg.ServiceName)
	log.Printf("Debug mode: %v", cfg.Debug)
	log.Printf("Max connections: %d", cfg.MaxConnections)
	log.Printf("Cache timeout: %s", cfg.CacheTimeout)
	log.Printf("Features enabled: %v", cfg.Features)

	// Run the application
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
