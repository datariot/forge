// Package config provides configuration management for Forge microservices.
//
// The config package implements environment-based configuration with comprehensive
// validation, sensible defaults, and support for multiple deployment environments.
//
// # Basic Usage
//
// Embed BaseConfig in your service-specific configuration:
//
//	type MyServiceConfig struct {
//		config.BaseConfig `yaml:",inline"`
//
//		// Service-specific fields
//		DatabaseURL string `yaml:"database_url" env:"DATABASE_URL"`
//		APIKey      string `yaml:"api_key" env:"API_KEY"`
//	}
//
//	func LoadConfig() (*MyServiceConfig, error) {
//		cfg := &MyServiceConfig{
//			BaseConfig: config.DefaultBaseConfig(),
//		}
//
//		// Load from environment, config files, etc.
//		// Then validate
//		if err := cfg.Validate(); err != nil {
//			return nil, err
//		}
//
//		return cfg, nil
//	}
//
// # Environment Variables
//
// All configuration fields can be set via environment variables.
// The package supports automatic environment variable binding with struct tags.
//
// # Validation
//
// The package provides comprehensive validation including:
//   - Required field validation
//   - URL format validation
//   - Address format validation
//   - Timeout and duration validation
//   - Environment-specific validation
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// BaseConfig contains configuration common to all services using the Forge framework.
// This should be embedded in service-specific configuration structs to inherit
// standard microservice configuration fields.
//
// BaseConfig provides:
//   - Service identification (name, environment, version)
//   - Server configuration (gRPC and HTTP addresses, timeouts)
//   - Logging configuration (level, format)
//   - Observability configuration (OpenTelemetry endpoints, sampling)
//   - Infrastructure URLs (database, Redis, etc.)
//   - Lifecycle configuration (shutdown timeouts, readiness delays)
//
// All fields support environment variable override using the env struct tag.
//
// Example environment variables:
//
//	SERVICE_NAME=user-service
//	APP_ENV=production
//	GRPC_ADDR=:8080
//	HTTP_ADDR=:8081
//	LOG_LEVEL=info
//	DATABASE_URL=postgres://localhost:5432/mydb
type BaseConfig struct {
	// ServiceName is the name of the service (e.g., "user-service", "auth-service")
	ServiceName string `yaml:"service_name" env:"SERVICE_NAME"`

	// AppEnv indicates the deployment environment (development, staging, production)
	AppEnv string `yaml:"app_env" env:"APP_ENV"`

	// GRPCAddr is the address to bind the gRPC server (e.g., ":8080")
	GRPCAddr string `yaml:"grpc_addr" env:"GRPC_ADDR"`

	// HTTPAddr is the address to bind the HTTP health/metrics server (e.g., ":8081")
	HTTPAddr string `yaml:"http_addr" env:"HTTP_ADDR"`

	// LogLevel controls the logging level (debug, info, warn, error)
	LogLevel string `yaml:"log_level" env:"LOG_LEVEL"`

	// ShutdownTimeout is the maximum time to wait for graceful shutdown
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`

	// ReadinessInitialDelay is the time to wait before marking the service as ready
	ReadinessInitialDelay time.Duration `yaml:"readiness_initial_delay" env:"READINESS_INITIAL_DELAY"`

	// Optional dependency URLs - only required if service uses them
	DatabaseURL string `yaml:"database_url" env:"DATABASE_URL"`
	RedisURL    string `yaml:"redis_url" env:"REDIS_URL"`

	// OpenTelemetry configuration
	OTELEndpoint   string  `yaml:"otel_endpoint" env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTELSampleRate float64 `yaml:"otel_sample_rate" env:"OTEL_SAMPLE_RATE"`

	// gRPC server configuration
	GRPCMaxRecvMsgSize int           `yaml:"grpc_max_recv_msg_size" env:"GRPC_MAX_RECV_MSG_SIZE"`
	GRPCMaxSendMsgSize int           `yaml:"grpc_max_send_msg_size" env:"GRPC_MAX_SEND_MSG_SIZE"`
	GRPCTimeout        time.Duration `yaml:"grpc_timeout" env:"GRPC_TIMEOUT"`

	// HTTP server configuration
	HTTPReadTimeout  time.Duration `yaml:"http_read_timeout" env:"HTTP_READ_TIMEOUT"`
	HTTPWriteTimeout time.Duration `yaml:"http_write_timeout" env:"HTTP_WRITE_TIMEOUT"`
	HTTPIdleTimeout  time.Duration `yaml:"http_idle_timeout" env:"HTTP_IDLE_TIMEOUT"`

	// Optional features
	EnablePprof      bool `yaml:"enable_pprof" env:"ENABLE_PPROF"`
	EnableReflection bool `yaml:"enable_reflection" env:"ENABLE_REFLECTION"`

	// HTTP server enhancements
	EnableCORS           bool     `yaml:"enable_cors" env:"ENABLE_CORS"`
	CORSOrigins          []string `yaml:"cors_origins" env:"CORS_ORIGINS"`
	EnableMetrics        bool     `yaml:"enable_metrics" env:"ENABLE_METRICS"`
	EnableRequestLogging bool     `yaml:"enable_request_logging" env:"ENABLE_REQUEST_LOGGING"`
}

// DefaultBaseConfig returns a BaseConfig with sensible defaults.
// Use this as a starting point for your service configuration, then override
// specific fields as needed.
//
// Default values include:
//   - ServiceName: "forge-service"
//   - AppEnv: "development"
//   - GRPCAddr: ":8080"
//   - HTTPAddr: ":8081"
//   - LogLevel: "info"
//   - ShutdownTimeout: 30 seconds
//   - OpenTelemetry sample rate: 1.0 (100%)
//
// Example usage:
//
//	cfg := config.DefaultBaseConfig()
//	cfg.ServiceName = "my-service"
//	cfg.AppEnv = "production"
func DefaultBaseConfig() BaseConfig {
	return BaseConfig{
		ServiceName:           "forge-service",
		AppEnv:                "development",
		GRPCAddr:              ":8080",
		HTTPAddr:              ":8081",
		LogLevel:              "info",
		ShutdownTimeout:       30 * time.Second,
		ReadinessInitialDelay: 0,
		OTELEndpoint:          "http://localhost:4317",
		OTELSampleRate:        1.0,
		GRPCMaxRecvMsgSize:    4 * 1024 * 1024, // 4MB
		GRPCMaxSendMsgSize:    4 * 1024 * 1024, // 4MB
		GRPCTimeout:           30 * time.Second,
		HTTPReadTimeout:       10 * time.Second,
		HTTPWriteTimeout:      10 * time.Second,
		HTTPIdleTimeout:       60 * time.Second,
		EnablePprof:           false,
		EnableReflection:      false, // Set to true in development via env/config
		EnableCORS:            false,
		CORSOrigins:           []string{},
		EnableMetrics:         true,
		EnableRequestLogging:  true,
	}
}

// Validate performs comprehensive validation on all BaseConfig fields.
// Returns an error if any validation fails, with a descriptive message
// indicating what needs to be corrected.
//
// Validation includes:
//   - Required fields (service name, environment, addresses)
//   - Valid log levels (debug, info, warn, error)
//   - Valid environments (development, staging, production)
//   - Positive timeouts and durations
//   - Valid URL formats for endpoints and infrastructure URLs
//   - Proper address formats (must contain port)
//   - OpenTelemetry sample rate between 0 and 1
//
// Call this method after loading configuration from environment or files
// to ensure all values are valid before starting the application.
func (c *BaseConfig) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}

	if c.AppEnv == "" {
		return fmt.Errorf("app_env is required")
	}

	if c.GRPCAddr == "" {
		return fmt.Errorf("grpc_addr is required")
	}

	if c.HTTPAddr == "" {
		return fmt.Errorf("http_addr is required")
	}

	// Validate log level
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// Valid levels
	default:
		return fmt.Errorf("invalid log_level '%s', must be one of: debug, info, warn, error", c.LogLevel)
	}

	// Validate environment
	switch c.AppEnv {
	case "development", "staging", "production":
		// Valid environments
	default:
		return fmt.Errorf("invalid app_env '%s', must be one of: development, staging, production", c.AppEnv)
	}

	// Validate timeouts are positive
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown_timeout must be positive, got %v", c.ShutdownTimeout)
	}

	if c.GRPCTimeout <= 0 {
		return fmt.Errorf("grpc_timeout must be positive, got %v", c.GRPCTimeout)
	}

	if c.HTTPReadTimeout <= 0 {
		return fmt.Errorf("http_read_timeout must be positive, got %v", c.HTTPReadTimeout)
	}

	if c.HTTPWriteTimeout <= 0 {
		return fmt.Errorf("http_write_timeout must be positive, got %v", c.HTTPWriteTimeout)
	}

	// Validate OTEL sample rate
	if c.OTELSampleRate < 0 || c.OTELSampleRate > 1 {
		return fmt.Errorf("otel_sample_rate must be between 0 and 1, got %f", c.OTELSampleRate)
	}

	// Validate OTEL endpoint URL format
	if c.OTELEndpoint != "" {
		if _, err := url.Parse(c.OTELEndpoint); err != nil {
			return fmt.Errorf("invalid otel_endpoint URL format: %w", err)
		}
	}

	// Validate address formats
	if !strings.Contains(c.GRPCAddr, ":") {
		return fmt.Errorf("invalid grpc_addr format '%s': must contain port (e.g., ':8080' or 'localhost:8080')", c.GRPCAddr)
	}

	if !strings.Contains(c.HTTPAddr, ":") {
		return fmt.Errorf("invalid http_addr format '%s': must contain port (e.g., ':8081' or 'localhost:8081')", c.HTTPAddr)
	}

	// Validate database URL format if provided
	if c.DatabaseURL != "" {
		if _, err := url.Parse(c.DatabaseURL); err != nil {
			return fmt.Errorf("invalid database_url format: %w", err)
		}
	}

	// Validate Redis URL format if provided
	if c.RedisURL != "" {
		if _, err := url.Parse(c.RedisURL); err != nil {
			return fmt.Errorf("invalid redis_url format: %w", err)
		}
	}

	return nil
}

// IsDevelopment returns true if the service is running in development mode.
func (c *BaseConfig) IsDevelopment() bool {
	return c.AppEnv == "development"
}

// IsProduction returns true if the service is running in production mode.
func (c *BaseConfig) IsProduction() bool {
	return c.AppEnv == "production"
}

// ShouldEnableReflection returns true if gRPC reflection should be enabled.
// By default, reflection is enabled in development unless explicitly disabled.
func (c *BaseConfig) ShouldEnableReflection() bool {
	if c.EnableReflection {
		return true
	}
	return c.IsDevelopment()
}

// Validator is an interface for configuration structs that can validate themselves.
type Validator interface {
	Validate() error
}
