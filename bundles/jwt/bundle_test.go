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

	if cfg.Issuer != "forge-service" {
		t.Errorf("Expected Issuer 'forge-service', got %s", cfg.Issuer)
	}

	if cfg.Audience != "forge-services" {
		t.Errorf("Expected Audience 'forge-services', got %s", cfg.Audience)
	}

	if cfg.TokenDuration != 1*time.Hour {
		t.Errorf("Expected TokenDuration 1h, got %v", cfg.TokenDuration)
	}

	if cfg.ClockSkew != 30*time.Second {
		t.Errorf("Expected ClockSkew 30s, got %v", cfg.ClockSkew)
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

	if bundle.Name() != "jwt" {
		t.Errorf("Expected bundle name 'jwt', got %s", bundle.Name())
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

	if err.Error() != "secret key is required" {
		t.Errorf("Expected 'secret key is required', got: %v", err)
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
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test-service"

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

// Note: validateServiceIdentifier is not exported, so we can't test it directly
// It's tested indirectly through Initialize() validation

// TestServiceIdentifierValidation tests service identifier validation indirectly
func TestServiceIdentifierValidation(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		wantErr    bool
	}{
		{"valid alphanumeric", "service123", false},
		{"valid with hyphens", "my-service", false},
		{"valid with underscores", "my_service", false},
		{"empty string", "", true},
		{"single character", "a", true},
		{"with spaces", "my service", true},
		{"with special chars", "service@123", true},
		{"with dots", "my.service", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test indirectly through config validation
			cfg := DefaultConfig()
			cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
			cfg.ServiceName = tt.identifier

			err := cfg.Validate()

			if tt.wantErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

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
