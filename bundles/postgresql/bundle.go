// Package postgresql provides a PostgreSQL integration bundle for Forge applications.
//
// The PostgreSQL bundle provides:
//   - Database connection management with connection pooling
//   - Health checks for database connectivity and performance
//   - Transaction utilities and patterns
//   - Graceful connection lifecycle management
//   - Production-ready connection pool configuration
//
// # Basic Usage
//
// Add the PostgreSQL bundle to your application:
//
//	config := postgresql.Config{
//		DatabaseURL: "postgres://user:pass@localhost/dbname?sslmode=require",
//		MaxOpenConns: 25,
//		MaxIdleConns: 10,
//		ConnMaxLifetime: 30 * time.Minute,
//	}
//
//	bundle := postgresql.NewBundle(config)
//
//	app, err := framework.New(
//		framework.WithConfig(&baseConfig),
//		framework.WithBundle(bundle),
//	)
//
// # Accessing the Database
//
// The bundle registers a database instance that can be accessed via dependency injection:
//
//	type UserService struct {
//		db *sql.DB
//	}
//
//	func NewUserService(db *sql.DB) *UserService {
//		return &UserService{db: db}
//	}
//
// # Health Checks
//
// The bundle automatically provides database health checks that verify:
//   - Database connectivity (ping)
//   - Query execution capability
//   - Connection pool health
//
// # Database Migrations
//
// The bundle focuses on runtime database connectivity. For database migrations,
// use dedicated tools during deployment:
//
//   - golang-migrate/migrate CLI tool
//   - Flyway for enterprise deployments
//   - Custom deployment scripts
//   - Kubernetes init containers
//
// This separation keeps the runtime bundle lightweight and follows the principle
// that migrations are a deployment concern, not a runtime concern.
package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/datariot/forge/forgeerrors"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// Config contains PostgreSQL-specific configuration options.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string.
	// Example: "postgres://user:password@localhost:5432/dbname?sslmode=disable"
	DatabaseURL string

	// Connection pool configuration
	MaxOpenConns    int           // Maximum number of open connections (default: 25)
	MaxIdleConns    int           // Maximum number of idle connections (default: 10)
	ConnMaxLifetime time.Duration // Maximum connection lifetime (default: 30 minutes)
	ConnMaxIdleTime time.Duration // Maximum connection idle time (default: 15 minutes)

	// Health check configuration
	HealthCheckTimeout time.Duration // Timeout for health check queries (default: 5 seconds)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxOpenConns:       25,
		MaxIdleConns:       10,
		ConnMaxLifetime:    30 * time.Minute,
		ConnMaxIdleTime:    15 * time.Minute,
		HealthCheckTimeout: 5 * time.Second,
	}
}

// Bundle provides PostgreSQL integration for Forge applications.
type Bundle struct {
	config Config
	db     *sql.DB
}

// NewBundle creates a new PostgreSQL bundle with the given configuration.
func NewBundle(config Config) *Bundle {
	return &Bundle{
		config: config,
	}
}

// Name returns the bundle name.
func (b *Bundle) Name() string {
	return "postgresql"
}

// Initialize sets up the PostgreSQL connection and performs migrations if configured.
func (b *Bundle) Initialize(app *framework.App) error {
	if b.config.DatabaseURL == "" {
		return forgeerrors.ErrInvalidConfiguration.WithMessage("database_url is required for PostgreSQL bundle")
	}

	// Validate connection pool configuration
	if b.config.MaxOpenConns <= 0 {
		return forgeerrors.ErrInvalidConfiguration.WithMessage("max_open_conns must be positive, got %d", b.config.MaxOpenConns)
	}
	if b.config.MaxIdleConns < 0 {
		return forgeerrors.ErrInvalidConfiguration.WithMessage("max_idle_conns must be non-negative, got %d", b.config.MaxIdleConns)
	}
	if b.config.MaxIdleConns > b.config.MaxOpenConns {
		return forgeerrors.ErrInvalidConfiguration.WithMessage("max_idle_conns (%d) cannot exceed max_open_conns (%d)", b.config.MaxIdleConns, b.config.MaxOpenConns)
	}
	if b.config.ConnMaxLifetime <= 0 {
		return forgeerrors.ErrInvalidConfiguration.WithMessage("conn_max_lifetime must be positive, got %v", b.config.ConnMaxLifetime)
	}
	if b.config.ConnMaxIdleTime < 0 {
		return forgeerrors.ErrInvalidConfiguration.WithMessage("conn_max_idle_time must be non-negative, got %v", b.config.ConnMaxIdleTime)
	}
	if b.config.HealthCheckTimeout <= 0 {
		return forgeerrors.ErrInvalidConfiguration.WithMessage("health_check_timeout must be positive, got %v", b.config.HealthCheckTimeout)
	}

	// Open database connection
	db, err := sql.Open("pgx", b.config.DatabaseURL)
	if err != nil {
		return forgeerrors.ErrRepositoryUnavailable.WithMessage("failed to open PostgreSQL connection").WithCause(err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(b.config.MaxOpenConns)
	db.SetMaxIdleConns(b.config.MaxIdleConns)
	db.SetConnMaxLifetime(b.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(b.config.ConnMaxIdleTime)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return forgeerrors.ErrRepositoryUnavailable.WithMessage("failed to ping PostgreSQL database").WithCause(err)
	}

	b.db = db

	return nil
}

// DB returns the database connection. This can be used for dependency injection.
func (b *Bundle) DB() *sql.DB {
	return b.db
}

// Stop implements the Bundle interface for graceful shutdown.
// Closes the database connection respecting the context deadline.
func (b *Bundle) Stop(ctx context.Context) error {
	if b.db == nil {
		return nil
	}

	// Channel to signal when connection is closed
	done := make(chan error, 1)
	go func() {
		done <- b.db.Close()
	}()

	// Wait for either successful closure or context timeout
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Force close after timeout
		if closeErr := b.db.Close(); closeErr != nil {
			return fmt.Errorf("database connection close timed out: %w (close error: %w)", ctx.Err(), closeErr)
		}
		return fmt.Errorf("database connection close timed out: %w", ctx.Err())
	}
}

// Close is deprecated. Use Stop() instead for proper lifecycle integration.
// Maintained for backward compatibility.
func (b *Bundle) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return b.Stop(ctx)
}

// HealthChecks returns health checks for the PostgreSQL connection.
func (b *Bundle) HealthChecks() []forgeHealth.Check {
	if b.db == nil {
		return nil
	}

	return []forgeHealth.Check{
		&PostgreSQLHealthCheck{
			db:      b.db,
			timeout: b.config.HealthCheckTimeout,
		},
	}
}

// PostgreSQLHealthCheck implements health checking for PostgreSQL connections.
type PostgreSQLHealthCheck struct {
	db      *sql.DB
	timeout time.Duration
}

// Name returns the health check name.
func (c *PostgreSQLHealthCheck) Name() string {
	return "postgresql"
}

// Liveness performs a basic connectivity check.
func (c *PostgreSQLHealthCheck) Liveness(ctx context.Context) error {
	// Use configured timeout, but respect parent context deadline
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Only create new timeout context if parent doesn't have a shorter deadline
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return c.db.PingContext(ctx)
}

// Readiness performs a more comprehensive check including query execution.
func (c *PostgreSQLHealthCheck) Readiness(ctx context.Context) error {
	// Use configured timeout, but respect parent context deadline
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Only create new timeout context if parent doesn't have a shorter deadline
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// First check basic connectivity
	if err := c.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Then verify we can execute a simple query
	var result int
	err := c.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("failed to execute test query: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected test query result: got %d, expected 1", result)
	}

	return nil
}
