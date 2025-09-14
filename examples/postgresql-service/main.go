// Package main demonstrates a Forge microservice with PostgreSQL integration.
//
// This example shows how to:
//   - Use the PostgreSQL bundle for database connectivity
//   - Implement database-backed business logic
//   - Provide database health checks
//   - Use proper transaction patterns
//   - Demonstrate connection pool management
//
// # Prerequisites
//
// 1. PostgreSQL server running locally
// 2. Database created: createdb forge_example
// 3. Database schema created using your preferred migration tool
//
// # Run the service
//
//   go run main.go
//
// # Environment Configuration
//
//   DATABASE_URL=postgres://user:pass@localhost:5432/forge_example?sslmode=disable
//   SERVICE_NAME=postgresql-service
//   APP_ENV=development
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/datariot/forge/bundles/postgresql"
	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// ServiceConfig extends BaseConfig with PostgreSQL-specific configuration.
type ServiceConfig struct {
	config.BaseConfig `yaml:",inline"`

	// PostgreSQL configuration
	DatabaseURL     string `yaml:"database_url" env:"DATABASE_URL"`
	MaxConnections  int    `yaml:"max_connections" env:"MAX_CONNECTIONS"`
}

// DefaultServiceConfig returns configuration with defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		BaseConfig:     config.DefaultBaseConfig(),
		MaxConnections: 25,
	}
}

// Validate validates the service configuration.
func (c *ServiceConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.DatabaseURL == "" {
		return fmt.Errorf("database_url is required")
	}

	return nil
}

// UserService demonstrates a database-backed service component.
type UserService struct {
	config *ServiceConfig
	db     *sql.DB
}

// NewUserService creates a new user service with database dependency.
func NewUserService(config *ServiceConfig, db *sql.DB) *UserService {
	return &UserService{
		config: config,
		db:     db,
	}
}

// Start initializes the user service.
func (s *UserService) Start(ctx context.Context) error {
	log.Printf("UserService started with database connection")

	// Example: Verify our tables exist (assumes schema is already created)
	if err := s.verifySchema(); err != nil {
		log.Printf("Warning: Schema verification failed (database may need initialization): %v", err)
	}

	return nil
}

// Stop gracefully shuts down the user service.
func (s *UserService) Stop(ctx context.Context) error {
	log.Printf("UserService stopping...")
	return nil
}

// HealthChecks implements the HealthContributor interface.
func (s *UserService) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		&UserServiceHealthCheck{
			db: s.db,
		},
	}
}

// verifySchema checks that required tables exist (example business logic).
func (s *UserService) verifySchema() error {
	// Example query to verify schema exists
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'users'
		)`

	err := s.db.QueryRow(query).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check schema: %w", err)
	}

	if !exists {
		log.Println("Warning: 'users' table does not exist - please create schema using your migration tool")
	}

	return nil
}

// CreateUser demonstrates a database operation with proper error handling.
func (s *UserService) CreateUser(ctx context.Context, name, email string) (int64, error) {
	query := `INSERT INTO users (name, email, created_at) VALUES ($1, $2, $3) RETURNING id`

	var id int64
	err := s.db.QueryRowContext(ctx, query, name, email, time.Now()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}

	return id, nil
}

// GetUser demonstrates a read operation.
func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
	user := &User{}
	query := `SELECT id, name, email, created_at FROM users WHERE id = $1`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Name, &user.Email, &user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// User represents a user entity.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// UserServiceHealthCheck provides service-specific health checking.
type UserServiceHealthCheck struct {
	db *sql.DB
}

// Name returns the health check name.
func (c *UserServiceHealthCheck) Name() string {
	return "user-service"
}

// Liveness performs a basic service health check.
func (c *UserServiceHealthCheck) Liveness(ctx context.Context) error {
	// Simple connectivity check is sufficient for liveness
	return c.db.PingContext(ctx)
}

// Readiness performs a comprehensive service readiness check.
func (c *UserServiceHealthCheck) Readiness(ctx context.Context) error {
	// Check database connectivity
	if err := c.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database connectivity failed: %w", err)
	}

	// Check that we can perform a basic query on our main table
	var count int
	err := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users LIMIT 1").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to query users table: %w", err)
	}

	return nil
}

func main() {
	// Load configuration
	cfg := DefaultServiceConfig()
	cfg.ServiceName = "postgresql-service"

	// Validate that database URL is provided
	if cfg.DatabaseURL == "" {
		log.Fatalf("DATABASE_URL environment variable is required. Example: postgres://user:pass@localhost:5432/dbname?sslmode=require")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Create PostgreSQL bundle
	pgConfig := postgresql.Config{
		DatabaseURL:        cfg.DatabaseURL,
		MaxOpenConns:       cfg.MaxConnections,
		MaxIdleConns:       cfg.MaxConnections / 2,
		ConnMaxLifetime:    30 * time.Minute,
		HealthCheckTimeout: 5 * time.Second,
	}

	pgBundle := postgresql.NewBundle(pgConfig)

	// Create the application using Forge framework
	app, err := framework.New(
		framework.WithConfig(&cfg.BaseConfig),
		framework.WithVersion("1.0.0"),
		framework.WithBundle(pgBundle),
		framework.WithStartupHook(func(ctx context.Context, app *framework.App) error {
			// Create user service with database dependency
			userService := NewUserService(&cfg, pgBundle.DB())

			// Register the user service as a component
			// Note: This is a simplified example - in practice you might use
			// a dependency injection container or service registry
			return userService.Start(ctx)
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Run the application
	log.Printf("Starting %s with PostgreSQL integration...", cfg.ServiceName)
	log.Printf("Note: Database schema should be created using migration tools during deployment")
	log.Printf("Example: golang-migrate -path ./migrations -database %s up",
		"postgres://user:pass@localhost:5432/dbname")

	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}