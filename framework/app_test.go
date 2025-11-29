package framework

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/datariot/forge/config"
	forgeHealth "github.com/datariot/forge/health"
)

// TestComponent is a test implementation of the Component interface
type TestComponent struct {
	started bool
	stopped bool
	startError error
	stopError error
}

func (c *TestComponent) Start(ctx context.Context) error {
	c.started = true
	return c.startError
}

func (c *TestComponent) Stop(ctx context.Context) error {
	c.stopped = true
	return c.stopError
}

func (c *TestComponent) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		forgeHealth.NewAlwaysHealthyCheck("test-component"),
	}
}

// TestBundle is a test implementation of the Bundle interface
type TestBundle struct {
	name string
	initError error
	initialized bool
	stopped bool
	stopError error
}

func (b *TestBundle) Name() string {
	return b.name
}

func (b *TestBundle) Initialize(app *App) error {
	b.initialized = true
	return b.initError
}

func (b *TestBundle) Stop(ctx context.Context) error {
	b.stopped = true
	return b.stopError
}

func TestApp_New_ValidConfig(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	app, err := New(
		WithConfig(&cfg),
		WithVersion("1.0.0"),
	)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if app == nil {
		t.Fatal("Expected app to be created")
	}

	if app.config.ServiceName != "test-service" {
		t.Errorf("Expected service name 'test-service', got %s", app.config.ServiceName)
	}

	if app.version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got %s", app.version)
	}
}

func TestApp_New_MissingConfig(t *testing.T) {
	_, err := New(
		WithVersion("1.0.0"),
	)

	if err == nil {
		t.Fatal("Expected error for missing config")
	}

	expectedError := "config is required"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestApp_WithComponent_ValidComponent(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	component := &TestComponent{}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(component),
	)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(app.components) != 1 {
		t.Fatalf("Expected 1 component, got %d", len(app.components))
	}

	if app.components[0] != component {
		t.Error("Expected component to be registered")
	}
}

func TestApp_WithComponent_NilComponent(t *testing.T) {
	cfg := config.DefaultBaseConfig()

	_, err := New(
		WithConfig(&cfg),
		WithComponent(nil),
	)

	if err == nil {
		t.Fatal("Expected error for nil component")
	}

	if err == nil {
		t.Fatal("Expected error for nil component")
	}

	if !strings.Contains(err.Error(), "component cannot be nil") {
		t.Errorf("Expected error to contain 'component cannot be nil', got '%s'", err.Error())
	}
}

func TestApp_WithBundle_ValidBundle(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	bundle := &TestBundle{name: "test-bundle"}

	app, err := New(
		WithConfig(&cfg),
		WithBundle(bundle),
	)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(app.bundles) != 1 {
		t.Fatalf("Expected 1 bundle, got %d", len(app.bundles))
	}

	if app.bundles[0] != bundle {
		t.Error("Expected bundle to be registered")
	}
}

func TestApp_Start_ComponentLifecycle(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"
	cfg.GRPCAddr = ":0" // Use random port
	cfg.HTTPAddr = ":0" // Use random port

	component := &TestComponent{}
	bundle := &TestBundle{name: "test-bundle"}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(component),
		WithBundle(bundle),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Test Start
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Verify bundle was initialized
	if !bundle.initialized {
		t.Error("Expected bundle to be initialized")
	}

	// Verify component was started
	if !component.started {
		t.Error("Expected component to be started")
	}

	// Test Stop
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	// Verify component was stopped
	if !component.stopped {
		t.Error("Expected component to be stopped")
	}
}

func TestApp_Start_BundleInitializationError(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	bundle := &TestBundle{
		name: "failing-bundle",
		initError: fmt.Errorf("initialization failed"),
	}

	app, err := New(
		WithConfig(&cfg),
		WithBundle(bundle),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.Start(ctx)
	if err == nil {
		t.Fatal("Expected error from bundle initialization failure")
	}

	if !strings.Contains(err.Error(), "failed to initialize bundle failing-bundle") {
		t.Errorf("Expected bundle initialization error, got: %v", err)
	}
}

func TestApp_Start_ComponentStartError(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	component := &TestComponent{
		startError: fmt.Errorf("component start failed"),
	}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(component),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.Start(ctx)
	if err == nil {
		t.Fatal("Expected error from component start failure")
	}

	if !strings.Contains(err.Error(), "failed to start component") {
		t.Errorf("Expected component start error, got: %v", err)
	}
}

func TestApp_HealthRegistry_Integration(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	component := &TestComponent{}

	app, err := New(
		WithConfig(&cfg),
		WithHealthContributor(component),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	registry := app.HealthRegistry()
	if registry == nil {
		t.Fatal("Expected health registry to be available")
	}

	// Health checks should be registered during startup
	// This would be tested in integration tests with actual startup
}

func TestApp_IsRunning_States(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Initially not running
	if app.IsRunning() {
		t.Error("Expected app to not be running initially")
	}

	// Would test running state after Start() in integration tests
}

func TestApp_Uptime_Calculation(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Uptime should be calculated from start time
	uptime := app.Uptime()
	if uptime <= 0 {
		t.Error("Expected positive uptime")
	}

	// Wait a bit and check uptime increased
	time.Sleep(10 * time.Millisecond)
	newUptime := app.Uptime()
	if newUptime <= uptime {
		t.Error("Expected uptime to increase")
	}
}