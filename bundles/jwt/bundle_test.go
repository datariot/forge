package jwt

import (
	"context"
	"testing"
	"time"

	"github.com/datariot/forge/errors"
)

// TestDefaultConfig tests default configuration values
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Note: Issuer, Audience, ServiceName, SecretKey are required to be set by user
	// DefaultConfig only sets the timing and security defaults

	if cfg.TokenDuration != 1*time.Hour {
		t.Errorf("Expected TokenDuration 1h, got %v", cfg.TokenDuration)
	}

	if cfg.ClockSkew != 1*time.Minute {
		t.Errorf("Expected ClockSkew 1m, got %v", cfg.ClockSkew)
	}

	if !cfg.RequireHTTPS {
		t.Error("Expected RequireHTTPS to be true by default (secure by default)")
	}

	if len(cfg.SkipPaths) != 1 || cfg.SkipPaths[0] != "/health" {
		t.Error("Expected SkipPaths to contain only /health by default")
	}
}

// TestNewBundle tests bundle creation
func TestNewBundle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test-service"

	bundle := NewBundle(cfg)

	if bundle == nil {
		t.Fatal("Expected bundle to be created")
	}

	if string(bundle.config.SecretKey) != string(cfg.SecretKey) {
		t.Error("Expected config to be set")
	}
}

// TestBundle_Name tests bundle name
func TestBundle_Name(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	if bundle.Name() != "jwt-auth" {
		t.Errorf("Expected bundle name 'jwt-auth', got %s", bundle.Name())
	}
}

// TestConfig_Validate_MissingSecretKey tests validation fails without secret key
func TestConfig_Validate_MissingSecretKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = nil

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing secret key")
	}

	if err.Error() != "jwt secret key is required" {
		t.Errorf("Expected 'jwt secret key is required', got: %v", err)
	}
}

// TestConfig_Validate_ShortSecretKey tests validation fails with short key
func TestConfig_Validate_ShortSecretKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("short")

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for short secret key")
	}
}

// TestConfig_Validate_MissingIssuer tests validation fails without issuer
func TestConfig_Validate_MissingIssuer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.Issuer = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing issuer")
	}
}

// TestConfig_Validate_MissingAudience tests validation fails without audience
func TestConfig_Validate_MissingAudience(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.Audience = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing audience")
	}
}

// TestConfig_Validate_MissingServiceName tests validation fails without service name
func TestConfig_Validate_MissingServiceName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing service name")
	}
}

// TestConfig_Validate_InvalidTokenDuration tests validation fails with invalid duration
func TestConfig_Validate_InvalidTokenDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test"
	cfg.TokenDuration = -1 * time.Second

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for negative token duration")
	}
}

// TestConfig_Validate_InvalidClockSkew tests validation fails with negative clock skew
func TestConfig_Validate_InvalidClockSkew(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test"
	cfg.ClockSkew = -1 * time.Second

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for negative clock skew")
	}
}

// TestConfig_Validate_Success tests successful validation
func TestConfig_Validate_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("this-is-a-32-byte-secret-key!!!!") // Exactly 32 bytes
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass, got: %v", err)
	}
}

// TestBundle_Initialize_InvalidConfig tests initialization fails with invalid config
func TestBundle_Initialize_InvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = nil // Invalid

	bundle := NewBundle(cfg)
	err := bundle.Initialize(nil)

	if err == nil {
		t.Fatal("Expected error for invalid configuration")
	}

	if !errors.IsConfigurationError(err) {
		t.Error("Expected configuration error")
	}
}

// TestBundle_Stop tests Stop method
func TestBundle_Stop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test-service"

	bundle := NewBundle(cfg)

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop to succeed, got: %v", err)
	}
}

// Note: Service identifier format validation (alphanumeric check) happens during
// token generation/validation, not during config validation. Config validation only
// checks that required fields are present.

// TestServiceClaims_Validation tests claims structure
func TestServiceClaims_Validation(t *testing.T) {
	claims := &ServiceClaims{
		ServiceID:   "svc-123",
		ServiceName: "test-service",
		Permissions: []string{"read", "write"},
	}

	if claims.ServiceID != "svc-123" {
		t.Error("ServiceID not set correctly")
	}

	if claims.ServiceName != "test-service" {
		t.Error("ServiceName not set correctly")
	}

	if len(claims.Permissions) != 2 {
		t.Error("Permissions not set correctly")
	}
}
