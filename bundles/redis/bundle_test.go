package redis

import (
	"context"
	"testing"
	"time"

	"github.com/datariot/forge/errors"
)

// TestDefaultConfig tests default configuration values
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PoolSize != 10 {
		t.Errorf("Expected PoolSize 10, got %d", cfg.PoolSize)
	}

	if cfg.MinIdleConns != 2 {
		t.Errorf("Expected MinIdleConns 2, got %d", cfg.MinIdleConns)
	}

	if cfg.MaxIdleTime != 30*time.Minute {
		t.Errorf("Expected MaxIdleTime 30m, got %v", cfg.MaxIdleTime)
	}

	if cfg.DialTimeout != 5*time.Second {
		t.Errorf("Expected DialTimeout 5s, got %v", cfg.DialTimeout)
	}

	if cfg.ReadTimeout != 3*time.Second {
		t.Errorf("Expected ReadTimeout 3s, got %v", cfg.ReadTimeout)
	}

	if cfg.WriteTimeout != 3*time.Second {
		t.Errorf("Expected WriteTimeout 3s, got %v", cfg.WriteTimeout)
	}
}

// TestNewBundle tests bundle creation
func TestNewBundle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RedisURL = "redis://localhost:6379/0"

	bundle := NewBundle(cfg)

	if bundle == nil {
		t.Fatal("Expected bundle to be created")
	}

	if bundle.config.RedisURL != cfg.RedisURL {
		t.Error("Expected config to be set")
	}
}

// TestBundle_Name tests bundle name
func TestBundle_Name(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	if bundle.Name() != "redis" {
		t.Errorf("Expected bundle name 'redis', got %s", bundle.Name())
	}
}

// TestBundle_Initialize_MissingRedisURL tests initialization fails without URL
func TestBundle_Initialize_MissingRedisURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RedisURL = "" // Missing URL

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for missing Redis URL")
	}

	if !errors.IsConfigurationError(err) {
		t.Error("Expected configuration error")
	}
}

// TestBundle_Initialize_InvalidPoolSize tests validation
func TestBundle_Initialize_InvalidPoolSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RedisURL = "redis://localhost:6379/0"
	cfg.PoolSize = 0 // Invalid

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for invalid PoolSize")
	}
}

// TestBundle_Initialize_InvalidMinIdleConns tests validation
func TestBundle_Initialize_InvalidMinIdleConns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RedisURL = "redis://localhost:6379/0"
	cfg.MinIdleConns = -1 // Invalid

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for negative MinIdleConns")
	}
}

// TestBundle_Initialize_MinIdleExceedsPoolSize tests validation
func TestBundle_Initialize_MinIdleExceedsPoolSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RedisURL = "redis://localhost:6379/0"
	cfg.PoolSize = 5
	cfg.MinIdleConns = 10 // Exceeds pool size

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error when MinIdleConns > PoolSize")
	}
}

// TestBundle_Initialize_InvalidTimeouts tests timeout validation
func TestBundle_Initialize_InvalidTimeouts(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
	}{
		{
			name:   "negative DialTimeout",
			modify: func(c *Config) { c.DialTimeout = -1 * time.Second },
		},
		{
			name:   "negative ReadTimeout",
			modify: func(c *Config) { c.ReadTimeout = -1 * time.Second },
		},
		{
			name:   "negative WriteTimeout",
			modify: func(c *Config) { c.WriteTimeout = -1 * time.Second },
		},
		{
			name:   "zero HealthCheckTimeout",
			modify: func(c *Config) { c.HealthCheckTimeout = 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.RedisURL = "redis://localhost:6379/0"
			tt.modify(&cfg)

			bundle := NewBundle(cfg)
			err := bundle.Initialize(nil)

			if err == nil {
				t.Errorf("Expected error for %s", tt.name)
			}
		})
	}
}

// TestConfig_SanitizedRedisURL tests URL sanitization
func TestConfig_SanitizedRedisURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "no password",
			url:      "redis://localhost:6379/0",
			expected: "redis://localhost:6379/0",
		},
		{
			name:     "with password",
			url:      "redis://user:secret123@localhost:6379/0",
			expected: "redis://user:***@localhost:6379/0",
		},
		{
			name:     "with TLS and password",
			url:      "rediss://user:password@redis.example.com:6380/0",
			expected: "rediss://user:***@redis.example.com:6380/0",
		},
		{
			name:     "invalid URL",
			url:      "not-a-valid-url",
			expected: "[invalid URL]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{RedisURL: tt.url}
			sanitized := cfg.SanitizedRedisURL()

			if sanitized != tt.expected {
				t.Errorf("Expected sanitized URL '%s', got '%s'", tt.expected, sanitized)
			}
		})
	}
}

// TestBundle_Stop_NilClient tests Stop with nil client
func TestBundle_Stop_NilClient(t *testing.T) {
	bundle := NewBundle(DefaultConfig())
	// client is nil, never initialized

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop with nil client to succeed, got: %v", err)
	}
}

// TestBundle_Client_ReturnsClient tests Client getter
func TestBundle_Client_ReturnsClient(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	// Before initialization
	if bundle.Client() != nil {
		t.Error("Expected Client to be nil before initialization")
	}
}

// TestBundle_Cache_ReturnsCache tests Cache getter
func TestBundle_Cache_ReturnsCache(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	// Before initialization
	if bundle.Cache() != nil {
		t.Error("Expected Cache to be nil before initialization")
	}
}

// TestBundle_PubSub_ReturnsPubSub tests PubSub getter
func TestBundle_PubSub_ReturnsPubSub(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	// Before initialization
	if bundle.PubSub() != nil {
		t.Error("Expected PubSub to be nil before initialization")
	}
}

// TestBundle_HealthChecks_NilClient tests HealthChecks with nil client
func TestBundle_HealthChecks_NilClient(t *testing.T) {
	bundle := NewBundle(DefaultConfig())
	// client is nil, never initialized

	checks := bundle.HealthChecks()
	if checks != nil {
		t.Error("Expected no health checks when client is nil")
	}
}

// TestRedisHealthCheck_Name tests health check name
func TestRedisHealthCheck_Name(t *testing.T) {
	check := &RedisHealthCheck{
		client:  nil,
		timeout: 5 * time.Second,
	}

	if check.Name() != "redis" {
		t.Errorf("Expected health check name 'redis', got %s", check.Name())
	}
}

// TestConfig_Validate_EdgeCases tests configuration validation edge cases
func TestConfig_Validate_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "missing RedisURL",
			modify:  func(c *Config) { c.RedisURL = "" },
			wantErr: true,
		},
		{
			name:    "zero PoolSize",
			modify:  func(c *Config) { c.PoolSize = 0 },
			wantErr: true,
		},
		{
			name:    "negative PoolSize",
			modify:  func(c *Config) { c.PoolSize = -1 },
			wantErr: true,
		},
		{
			name:    "negative MinIdleConns",
			modify:  func(c *Config) { c.MinIdleConns = -1 },
			wantErr: true,
		},
		{
			name: "MinIdleConns exceeds PoolSize",
			modify: func(c *Config) {
				c.PoolSize = 5
				c.MinIdleConns = 10
			},
			wantErr: true,
		},
		{
			name:    "valid config",
			modify:  func(c *Config) { c.RedisURL = "redis://localhost:6379" },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(&cfg)

			bundle := NewBundle(cfg)
			err := bundle.Initialize(nil)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got nil")
			}
			// Note: "valid config" will fail without actual Redis, but validates config logic
			if !tt.wantErr && err != nil && !errors.IsRepositoryError(err) {
				// Only fail if it's NOT a repository error (connection failure is expected)
				t.Errorf("Expected no validation error but got: %v", err)
			}
		})
	}
}
