package main

import (
	"context"
	"log"

	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	"github.com/datariot/forge/health"
)

// SimpleConfig extends BaseConfig with service-specific configuration
type SimpleConfig struct {
	config.BaseConfig `yaml:",inline"`

	// Service-specific configuration
	Message string `yaml:"message" env:"MESSAGE"`
}

// DefaultSimpleConfig returns configuration with defaults
func DefaultSimpleConfig() SimpleConfig {
	return SimpleConfig{
		BaseConfig: config.DefaultBaseConfig(),
		Message:    "Hello from Forge!",
	}
}

// Validate validates the configuration
func (c *SimpleConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	// Add service-specific validation here
	return nil
}

// SimpleComponent implements the framework interfaces
type SimpleComponent struct {
	config *SimpleConfig
}

// NewSimpleComponent creates a new simple component
func NewSimpleComponent(config *SimpleConfig) *SimpleComponent {
	return &SimpleComponent{
		config: config,
	}
}

// Start implements the Component interface
func (c *SimpleComponent) Start(ctx context.Context) error {
	log.Printf("SimpleComponent started with message: %s", c.config.Message)
	return nil
}

// Stop implements the Component interface
func (c *SimpleComponent) Stop(ctx context.Context) error {
	log.Printf("SimpleComponent stopping...")
	return nil
}

// HealthChecks implements the HealthContributor interface
func (c *SimpleComponent) HealthChecks() []health.Check {
	return []health.Check{
		health.NewAlwaysHealthyCheck("simple-component"),
	}
}

func main() {
	// Load configuration (would typically read from environment/config files)
	cfg := DefaultSimpleConfig()
	cfg.ServiceName = "simple-service"

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Create the service component
	component := NewSimpleComponent(&cfg)

	// Create the application using Forge framework
	app, err := framework.New(
		framework.WithConfig(&cfg.BaseConfig),
		framework.WithVersion("1.0.0"),
		framework.WithComponent(component),
		framework.WithHealthContributor(component),
	)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Run the application (this will block until shutdown)
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}