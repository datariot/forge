package postgresql

import (
	"context"
	"testing"
	"time"

	"github.com/datariot/forge/errors"
)

// TestDefaultConfig tests default configuration values
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxOpenConns != 25 {
		t.Errorf("Expected MaxOpenConns 25, got %d", cfg.MaxOpenConns)
	}

	if cfg.MaxIdleConns != 10 {
		t.Errorf("Expected MaxIdleConns 10, got %d", cfg.MaxIdleConns)
	}

	if cfg.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("Expected ConnMaxLifetime 30m, got %v", cfg.ConnMaxLifetime)
	}

	if cfg.ConnMaxIdleTime != 15*time.Minute {
		t.Errorf("Expected ConnMaxIdleTime 15m, got %v", cfg.ConnMaxIdleTime)
	}

	if cfg.HealthCheckTimeout != 5*time.Second {
		t.Errorf("Expected HealthCheckTimeout 5s, got %v", cfg.HealthCheckTimeout)
	}
}

// TestNewBundle tests bundle creation
func TestNewBundle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabaseURL = "postgres://localhost:5432/testdb"

	bundle := NewBundle(cfg)

	if bundle == nil {
		t.Fatal("Expected bundle to be created")
	}

	if bundle.config.DatabaseURL != cfg.DatabaseURL {
		t.Error("Expected config to be set")
	}
}

// TestBundle_Name tests bundle name
func TestBundle_Name(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	if bundle.Name() != "postgresql" {
		t.Errorf("Expected bundle name 'postgresql', got %s", bundle.Name())
	}
}

// TestBundle_Initialize_MissingDatabaseURL tests initialization fails without URL
func TestBundle_Initialize_MissingDatabaseURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabaseURL = "" // Missing URL

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for missing database URL")
	}

	if !errors.IsConfigurationError(err) {
		t.Error("Expected configuration error")
	}
}

// TestBundle_Initialize_InvalidMaxOpenConns tests validation
func TestBundle_Initialize_InvalidMaxOpenConns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabaseURL = "postgres://localhost:5432/testdb"
	cfg.MaxOpenConns = 0 // Invalid

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for invalid MaxOpenConns")
	}

	if !errors.IsConfigurationError(err) {
		t.Error("Expected configuration error")
	}
}

// TestBundle_Initialize_InvalidMaxIdleConns tests validation
func TestBundle_Initialize_InvalidMaxIdleConns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabaseURL = "postgres://localhost:5432/testdb"
	cfg.MaxIdleConns = -1 // Invalid

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for negative MaxIdleConns")
	}
}

// TestBundle_Initialize_MaxIdleExceedsMaxOpen tests validation
func TestBundle_Initialize_MaxIdleExceedsMaxOpen(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabaseURL = "postgres://localhost:5432/testdb"
	cfg.MaxIdleConns = 30
	cfg.MaxOpenConns = 25 // Idle > Open

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error when MaxIdleConns > MaxOpenConns")
	}
}

// TestBundle_Initialize_InvalidConnMaxLifetime tests validation
func TestBundle_Initialize_InvalidConnMaxLifetime(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabaseURL = "postgres://localhost:5432/testdb"
	cfg.ConnMaxLifetime = -1 * time.Second

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for negative ConnMaxLifetime")
	}
}

// TestBundle_Initialize_InvalidHealthCheckTimeout tests validation
func TestBundle_Initialize_InvalidHealthCheckTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabaseURL = "postgres://localhost:5432/testdb"
	cfg.HealthCheckTimeout = 0

	bundle := NewBundle(cfg)

	err := bundle.Initialize(nil)
	if err == nil {
		t.Fatal("Expected error for zero HealthCheckTimeout")
	}
}

// TestBundle_Stop_NilDB tests Stop with nil database
func TestBundle_Stop_NilDB(t *testing.T) {
	bundle := NewBundle(DefaultConfig())
	// db is nil, never initialized

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop with nil db to succeed, got: %v", err)
	}
}

// TestBundle_DB_ReturnsConnection tests DB getter
func TestBundle_DB_ReturnsConnection(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	// Before initialization
	if bundle.DB() != nil {
		t.Error("Expected DB to be nil before initialization")
	}
}

// TestBundle_HealthChecks_NilDB tests HealthChecks with nil database
func TestBundle_HealthChecks_NilDB(t *testing.T) {
	bundle := NewBundle(DefaultConfig())
	// db is nil, never initialized

	checks := bundle.HealthChecks()
	if checks != nil {
		t.Error("Expected no health checks when DB is nil")
	}
}

// TestPostgreSQLHealthCheck_Name tests health check name
func TestPostgreSQLHealthCheck_Name(t *testing.T) {
	check := &PostgreSQLHealthCheck{
		db:      nil,
		timeout: 5 * time.Second,
	}

	if check.Name() != "postgresql" {
		t.Errorf("Expected health check name 'postgresql', got %s", check.Name())
	}
}

// TestConfig_Validation_AllFieldsValid tests config validation (not actual connection)
func TestConfig_Validation_AllFieldsValid(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "zero MaxOpenConns",
			modify:  func(c *Config) { c.MaxOpenConns = 0 },
			wantErr: true,
		},
		{
			name:    "negative MaxIdleConns",
			modify:  func(c *Config) { c.MaxIdleConns = -1 },
			wantErr: true,
		},
		{
			name: "MaxIdleConns exceeds MaxOpenConns",
			modify: func(c *Config) {
				c.MaxOpenConns = 10
				c.MaxIdleConns = 20
			},
			wantErr: true,
		},
		{
			name:    "zero ConnMaxLifetime",
			modify:  func(c *Config) { c.ConnMaxLifetime = 0 },
			wantErr: true,
		},
		{
			name:    "negative ConnMaxIdleTime",
			modify:  func(c *Config) { c.ConnMaxIdleTime = -1 * time.Second },
			wantErr: true,
		},
		{
			name:    "zero HealthCheckTimeout",
			modify:  func(c *Config) { c.HealthCheckTimeout = 0 },
			wantErr: true,
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
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
