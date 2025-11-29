package config

import (
	"testing"
	"time"
)

// TestDefaultBaseConfig tests that default config has sensible values
func TestDefaultBaseConfig(t *testing.T) {
	cfg := DefaultBaseConfig()

	if cfg.ServiceName != "forge-service" {
		t.Errorf("Expected default service name 'forge-service', got %s", cfg.ServiceName)
	}

	if cfg.AppEnv != "development" {
		t.Errorf("Expected default environment 'development', got %s", cfg.AppEnv)
	}

	if cfg.GRPCAddr != ":8080" {
		t.Errorf("Expected default gRPC address ':8080', got %s", cfg.GRPCAddr)
	}

	if cfg.HTTPAddr != ":8081" {
		t.Errorf("Expected default HTTP address ':8081', got %s", cfg.HTTPAddr)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("Expected default log level 'info', got %s", cfg.LogLevel)
	}

	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("Expected default shutdown timeout 30s, got %v", cfg.ShutdownTimeout)
	}

	if cfg.OTELSampleRate != 1.0 {
		t.Errorf("Expected default OTEL sample rate 1.0, got %f", cfg.OTELSampleRate)
	}
}

// TestBaseConfig_Validate_Success tests successful validation
func TestBaseConfig_Validate_Success(t *testing.T) {
	cfg := DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected validation to pass, got error: %v", err)
	}
}

// TestBaseConfig_Validate_MissingServiceName tests validation fails without service name
func TestBaseConfig_Validate_MissingServiceName(t *testing.T) {
	cfg := DefaultBaseConfig()
	cfg.ServiceName = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for missing service name")
	}

	if err.Error() != "service_name is required" {
		t.Errorf("Expected 'service_name is required' error, got: %v", err)
	}
}

// TestBaseConfig_Validate_InvalidLogLevel tests validation fails with invalid log level
func TestBaseConfig_Validate_InvalidLogLevel(t *testing.T) {
	cfg := DefaultBaseConfig()
	cfg.LogLevel = "invalid"

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid log level")
	}
}

// TestBaseConfig_Validate_InvalidEnvironment tests validation fails with invalid environment
func TestBaseConfig_Validate_InvalidEnvironment(t *testing.T) {
	cfg := DefaultBaseConfig()
	cfg.AppEnv = "invalid"

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid environment")
	}
}

// TestBaseConfig_Validate_NegativeTimeout tests validation fails with negative timeout
func TestBaseConfig_Validate_NegativeTimeout(t *testing.T) {
	cfg := DefaultBaseConfig()
	cfg.ShutdownTimeout = -1 * time.Second

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for negative shutdown timeout")
	}
}

// TestBaseConfig_Validate_InvalidSampleRate tests validation fails with invalid sample rate
func TestBaseConfig_Validate_InvalidSampleRate(t *testing.T) {
	tests := []struct {
		name string
		rate float64
	}{
		{"negative", -0.1},
		{"greater than 1", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultBaseConfig()
			cfg.OTELSampleRate = tt.rate

			err := cfg.Validate()
			if err == nil {
				t.Errorf("Expected validation error for sample rate %f", tt.rate)
			}
		})
	}
}

// TestBaseConfig_Validate_InvalidAddress tests validation fails with invalid address formats
func TestBaseConfig_Validate_InvalidAddress(t *testing.T) {
	tests := []struct {
		name  string
		grpc  string
		http  string
		field string
	}{
		{"missing grpc port", "localhost", ":8081", "grpc"},
		{"missing http port", ":8080", "localhost", "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultBaseConfig()
			cfg.GRPCAddr = tt.grpc
			cfg.HTTPAddr = tt.http

			err := cfg.Validate()
			if err == nil {
				t.Errorf("Expected validation error for invalid %s address", tt.field)
			}
		})
	}
}

// TestBaseConfig_IsDevelopment tests environment detection
func TestBaseConfig_IsDevelopment(t *testing.T) {
	cfg := DefaultBaseConfig()

	cfg.AppEnv = "development"
	if !cfg.IsDevelopment() {
		t.Error("Expected IsDevelopment() to return true for development")
	}

	cfg.AppEnv = "production"
	if cfg.IsDevelopment() {
		t.Error("Expected IsDevelopment() to return false for production")
	}
}

// TestBaseConfig_IsProduction tests production detection
func TestBaseConfig_IsProduction(t *testing.T) {
	cfg := DefaultBaseConfig()

	cfg.AppEnv = "production"
	if !cfg.IsProduction() {
		t.Error("Expected IsProduction() to return true for production")
	}

	cfg.AppEnv = "development"
	if cfg.IsProduction() {
		t.Error("Expected IsProduction() to return false for development")
	}
}

// TestBaseConfig_ShouldEnableReflection tests reflection enablement logic
func TestBaseConfig_ShouldEnableReflection(t *testing.T) {
	tests := []struct {
		name             string
		env              string
		explicitEnable   bool
		expectedEnabled  bool
	}{
		{"development implicit", "development", false, true},
		{"production implicit", "production", false, false},
		{"production explicit", "production", true, true},
		{"development explicit", "development", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultBaseConfig()
			cfg.AppEnv = tt.env
			cfg.EnableReflection = tt.explicitEnable

			result := cfg.ShouldEnableReflection()
			if result != tt.expectedEnabled {
				t.Errorf("Expected ShouldEnableReflection() to return %v, got %v",
					tt.expectedEnabled, result)
			}
		})
	}
}
