package framework

import (
	"context"
	"testing"
	"time"

	"github.com/datariot/forge/config"
	forgeHealth "github.com/datariot/forge/health"
)

// TestApp_Lifecycle_StartAndStop tests basic app lifecycle
func TestApp_Lifecycle_StartAndStop(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "lifecycle-test"
	cfg.GRPCAddr = ":0" // Random port
	cfg.HTTPAddr = ":0" // Random port

	app, err := New(
		WithConfig(&cfg),
		WithVersion("test-1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Test initial state
	if app.IsRunning() {
		t.Error("App should not be running initially")
	}

	if app.IsStopping() {
		t.Error("App should not be stopping initially")
	}

	// Start the app
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Verify running state
	if !app.IsRunning() {
		t.Error("App should be running after Start()")
	}

	// Wait a moment for servers to start
	time.Sleep(100 * time.Millisecond)

	// Stop the app
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	// Verify stopped state
	if app.IsRunning() {
		t.Error("App should not be running after Stop()")
	}
}

// TestApp_Lifecycle_ComponentStartOrder tests components start in registration order
func TestApp_Lifecycle_ComponentStartOrder(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "component-order-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	startOrder := []string{}

	comp1 := &TestComponentWithCallbacks{
		startFn: func() error {
			startOrder = append(startOrder, "comp1")
			return nil
		},
	}

	comp2 := &TestComponentWithCallbacks{
		startFn: func() error {
			startOrder = append(startOrder, "comp2")
			return nil
		},
	}

	comp3 := &TestComponentWithCallbacks{
		startFn: func() error {
			startOrder = append(startOrder, "comp3")
			return nil
		},
	}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(comp1),
		WithComponent(comp2),
		WithComponent(comp3),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Verify start order
	expected := []string{"comp1", "comp2", "comp3"}
	if len(startOrder) != len(expected) {
		t.Fatalf("Expected %d components to start, got %d", len(expected), len(startOrder))
	}

	for i, name := range expected {
		if startOrder[i] != name {
			t.Errorf("Expected component %d to be %s, got %s", i, name, startOrder[i])
		}
	}

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	app.Stop(stopCtx)
}

// TestApp_Lifecycle_ComponentStopReverseOrder tests components stop in reverse order
func TestApp_Lifecycle_ComponentStopReverseOrder(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "component-stop-order-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	stopOrder := []string{}

	comp1 := &TestComponentWithCallbacks{
		stopFn: func() error {
			stopOrder = append(stopOrder, "comp1")
			return nil
		},
	}

	comp2 := &TestComponentWithCallbacks{
		stopFn: func() error {
			stopOrder = append(stopOrder, "comp2")
			return nil
		},
	}

	comp3 := &TestComponentWithCallbacks{
		stopFn: func() error {
			stopOrder = append(stopOrder, "comp3")
			return nil
		},
	}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(comp1),
		WithComponent(comp2),
		WithComponent(comp3),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Stop the app
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	// Verify stop order (reverse of registration)
	expected := []string{"comp3", "comp2", "comp1"}
	if len(stopOrder) != len(expected) {
		t.Fatalf("Expected %d components to stop, got %d", len(expected), len(stopOrder))
	}

	for i, name := range expected {
		if stopOrder[i] != name {
			t.Errorf("Expected component %d to be %s, got %s", i, name, stopOrder[i])
		}
	}
}

// TestApp_Lifecycle_BundleInitialization tests bundle initialization
func TestApp_Lifecycle_BundleInitialization(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "bundle-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	bundle := &TestBundle{
		name: "test-bundle",
	}

	app, err := New(
		WithConfig(&cfg),
		WithBundle(bundle),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	if bundle.initialized {
		t.Error("Bundle should not be initialized before Start()")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	if !bundle.initialized {
		t.Error("Bundle should be initialized after Start()")
	}

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	app.Stop(stopCtx)
}

// TestApp_Lifecycle_StartupHooks tests startup hook execution
func TestApp_Lifecycle_StartupHooks(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "startup-hook-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	hookCalled := false

	app, err := New(
		WithConfig(&cfg),
		WithStartupHook(func(ctx context.Context, app *App) error {
			hookCalled = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	if !hookCalled {
		t.Error("Startup hook should have been called")
	}

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	app.Stop(stopCtx)
}

// TestApp_Lifecycle_ShutdownHooks tests shutdown hook execution
func TestApp_Lifecycle_ShutdownHooks(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "shutdown-hook-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	hookCalled := false

	app, err := New(
		WithConfig(&cfg),
		WithShutdownHook(func(ctx context.Context, app *App) error {
			hookCalled = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Stop the app
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	if !hookCalled {
		t.Error("Shutdown hook should have been called")
	}
}

// TestApp_Lifecycle_HealthCheckRegistration tests health check registration
func TestApp_Lifecycle_HealthCheckRegistration(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "health-check-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	component := &TestComponentWithCallbacks{
		healthChecks: []forgeHealth.Check{
			forgeHealth.NewAlwaysHealthyCheck("test-component"),
		},
	}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(component),
		WithHealthContributor(component),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Check health after startup
	registry := app.HealthRegistry()
	if registry == nil {
		t.Fatal("Health registry should not be nil")
	}

	checkCtx, checkCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer checkCancel()

	status := registry.CheckLiveness(checkCtx)
	if status.Status != "healthy" {
		t.Errorf("Expected healthy status, got %s", status.Status)
	}

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	app.Stop(stopCtx)
}

// TestApp_Lifecycle_MultipleStartCallsFail tests that calling Start() twice fails
func TestApp_Lifecycle_MultipleStartCallsFail(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "multiple-start-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(
		WithConfig(&cfg),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app first time: %v", err)
	}

	// Try to start again
	if err := app.Start(ctx); err == nil {
		t.Error("Expected error when starting app twice, got nil")
	}

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	app.Stop(stopCtx)
}

// TestApp_Lifecycle_Uptime tests uptime tracking
func TestApp_Lifecycle_Uptime(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "uptime-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(
		WithConfig(&cfg),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Check initial uptime (should be very small)
	uptime1 := app.Uptime()
	if uptime1 > 1*time.Second {
		t.Errorf("Initial uptime too large: %v", uptime1)
	}

	// Wait and check again
	time.Sleep(100 * time.Millisecond)
	uptime2 := app.Uptime()

	if uptime2 <= uptime1 {
		t.Error("Uptime should increase over time")
	}

	if uptime2 < 100*time.Millisecond {
		t.Errorf("Uptime should be at least 100ms, got %v", uptime2)
	}
}

// Helper test component with callback functions
type TestComponentWithCallbacks struct {
	started      bool
	stopped      bool
	startError   error
	stopError    error
	startFn      func() error
	stopFn       func() error
	healthChecks []forgeHealth.Check
}

func (c *TestComponentWithCallbacks) Start(ctx context.Context) error {
	c.started = true
	if c.startFn != nil {
		return c.startFn()
	}
	return c.startError
}

func (c *TestComponentWithCallbacks) Stop(ctx context.Context) error {
	c.stopped = true
	if c.stopFn != nil {
		return c.stopFn()
	}
	return c.stopError
}

func (c *TestComponentWithCallbacks) HealthChecks() []forgeHealth.Check {
	return c.healthChecks
}
