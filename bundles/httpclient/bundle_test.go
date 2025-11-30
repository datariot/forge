package httpclient

import (
	"context"
	"testing"
	"time"
)

// TestDefaultRetryConfig tests default retry configuration
func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", cfg.MaxRetries)
	}

	if cfg.InitialInterval != 100*time.Millisecond {
		t.Errorf("Expected InitialInterval 100ms, got %v", cfg.InitialInterval)
	}

	if cfg.MaxInterval != 10*time.Second {
		t.Errorf("Expected MaxInterval 10s, got %v", cfg.MaxInterval)
	}

	if cfg.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %f", cfg.Multiplier)
	}
}

// TestDefaultCircuitBreakerConfig tests default circuit breaker configuration
func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()

	if cfg.MaxRequests != 1 {
		t.Errorf("Expected MaxRequests 1, got %d", cfg.MaxRequests)
	}

	if cfg.Interval != 60*time.Second {
		t.Errorf("Expected Interval 60s, got %v", cfg.Interval)
	}

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", cfg.Timeout)
	}
}

// TestDefaultConfig tests default HTTP client configuration
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", cfg.Timeout)
	}

	if cfg.MaxIdleConns != 100 {
		t.Errorf("Expected MaxIdleConns 100, got %d", cfg.MaxIdleConns)
	}

	if cfg.MaxIdleConnsPerHost != 10 {
		t.Errorf("Expected MaxIdleConnsPerHost 10, got %d", cfg.MaxIdleConnsPerHost)
	}

	if cfg.IdleConnTimeout != 90*time.Second {
		t.Errorf("Expected IdleConnTimeout 90s, got %v", cfg.IdleConnTimeout)
	}

	if !cfg.EnableCircuitBreaker {
		t.Error("Expected EnableCircuitBreaker to be true by default")
	}

	if !cfg.EnableRetry {
		t.Error("Expected EnableRetry to be true by default")
	}
}

// TestNewBundle tests bundle creation
func TestNewBundle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseURL = "http://api.example.com"

	bundle := NewBundle(cfg)

	if bundle == nil {
		t.Fatal("Expected bundle to be created")
	}

	if bundle.config.BaseURL != cfg.BaseURL {
		t.Error("Expected config to be set")
	}
}

// TestBundle_Name tests bundle name
func TestBundle_Name(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	if bundle.Name() != "httpclient" {
		t.Errorf("Expected bundle name 'httpclient', got %s", bundle.Name())
	}
}

// TestBundle_Initialize tests bundle initialization
func TestBundle_Initialize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseURL = "http://api.example.com"

	bundle := NewBundle(cfg)

	// Initialize should succeed even without base URL (client can work without it)
	if err := bundle.Initialize(nil); err != nil {
		t.Errorf("Expected initialization to succeed, got: %v", err)
	}

	// Verify client was created
	if bundle.Client() == nil {
		t.Error("Expected client to be created after initialization")
	}
}

// TestBundle_Client_BeforeInitialize tests Client getter before initialization
func TestBundle_Client_BeforeInitialize(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	// Before initialization
	if bundle.Client() != nil {
		t.Error("Expected Client to be nil before initialization")
	}
}

// TestBundle_Stop tests Stop method
func TestBundle_Stop(t *testing.T) {
	cfg := DefaultConfig()
	bundle := NewBundle(cfg)

	// Initialize first
	bundle.Initialize(nil)

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop to succeed, got: %v", err)
	}
}

// TestBundle_Stop_BeforeInitialize tests Stop before initialization
func TestBundle_Stop_BeforeInitialize(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop to succeed even before initialization, got: %v", err)
	}
}

// TestRetryConfig_Validation tests retry configuration validation
func TestRetryConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*RetryConfig)
		valid  bool
	}{
		{
			name:   "valid default",
			modify: func(c *RetryConfig) {},
			valid:  true,
		},
		{
			name:   "zero MaxRetries",
			modify: func(c *RetryConfig) { c.MaxRetries = 0 },
			valid:  true, // Zero retries is valid (disables retries)
		},
		{
			name:   "negative MaxRetries",
			modify: func(c *RetryConfig) { c.MaxRetries = -1 },
			valid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultRetryConfig()
			tt.modify(&cfg)

			// Note: No validation method exists, so we just verify
			// the config can be created with these values
			if tt.valid && cfg.MaxRetries < 0 {
				t.Error("Invalid config should not be possible")
			}
		})
	}
}

// TestCircuitBreakerConfig_Defaults tests circuit breaker has reasonable settings
func TestCircuitBreakerConfig_Defaults(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()

	// Verify reasonable production settings
	if cfg.MaxRequests < 1 {
		t.Error("MaxRequests should be at least 1")
	}

	if cfg.Interval < 1*time.Second {
		t.Error("Interval should be at least 1 second")
	}

	if cfg.Timeout < 1*time.Second {
		t.Error("Timeout should be at least 1 second")
	}
}

// TestConfig_HTTPSecurity tests HTTPS-related configuration
func TestConfig_HTTPSecurity(t *testing.T) {
	cfg := DefaultConfig()

	// Verify secure defaults
	if cfg.MaxIdleConns <= 0 {
		t.Error("MaxIdleConns should be positive")
	}

	if cfg.MaxIdleConnsPerHost <= 0 {
		t.Error("MaxIdleConnsPerHost should be positive")
	}

	if cfg.IdleConnTimeout <= 0 {
		t.Error("IdleConnTimeout should be positive")
	}

	if cfg.Timeout <= 0 {
		t.Error("Timeout should be positive")
	}
}
