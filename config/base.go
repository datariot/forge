package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// BaseConfig contains configuration common to all services using the Forge framework.
// This should be embedded in service-specific configuration structs.
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
}

// DefaultBaseConfig returns a BaseConfig with sensible defaults.
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
	}
}

// Validate performs validation on the BaseConfig fields.
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