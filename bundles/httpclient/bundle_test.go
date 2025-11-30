package httpclient

import (
	"context"
	"testing"
	"time"
)

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

	// Check retry config defaults
	if cfg.RetryConfig.MaxRetries != 3 {
		t.Errorf("Expected default MaxRetries 3, got %d", cfg.RetryConfig.MaxRetries)
	}

	if cfg.RetryConfig.InitialInterval != 100*time.Millisecond {
		t.Errorf("Expected default InitialInterval 100ms, got %v", cfg.RetryConfig.InitialInterval)
	}

	// Check circuit breaker defaults
	if cfg.CircuitBreakerConfig.MaxRequests != 3 {
		t.Errorf("Expected default CB MaxRequests 3, got %d", cfg.CircuitBreakerConfig.MaxRequests)
	}

	if cfg.CircuitBreakerConfig.Timeout != 30*time.Second {
		t.Errorf("Expected default CB Timeout 30s, got %v", cfg.CircuitBreakerConfig.Timeout)
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

	if bundle.Name() != "http-client" {
		t.Errorf("Expected bundle name 'http-client', got %s", bundle.Name())
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

// TestConfig_RetrySettings tests retry configuration embedded in Config
func TestConfig_RetrySettings(t *testing.T) {
	cfg := DefaultConfig()

	// Verify retry settings are initialized
	if cfg.RetryConfig.MaxRetries <= 0 {
		t.Error("Expected positive MaxRetries")
	}

	if cfg.RetryConfig.InitialInterval <= 0 {
		t.Error("Expected positive InitialInterval")
	}

	if cfg.RetryConfig.MaxInterval <= 0 {
		t.Error("Expected positive MaxInterval")
	}

	if cfg.RetryConfig.Multiplier <= 1.0 {
		t.Error("Expected Multiplier > 1.0 for exponential backoff")
	}
}

// TestConfig_CircuitBreakerSettings tests circuit breaker configuration
func TestConfig_CircuitBreakerSettings(t *testing.T) {
	cfg := DefaultConfig()

	// Verify circuit breaker settings are initialized
	if cfg.CircuitBreakerConfig.MaxRequests < 1 {
		t.Error("MaxRequests should be at least 1")
	}

	if cfg.CircuitBreakerConfig.Interval < 1*time.Second {
		t.Error("Interval should be at least 1 second")
	}

	if cfg.CircuitBreakerConfig.Timeout < 1*time.Second {
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
