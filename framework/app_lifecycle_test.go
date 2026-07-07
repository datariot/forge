package framework

import (
	"context"
	"runtime"
	"strings"
	"sync"
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
	if err := app.Stop(stopCtx); err != nil {
		t.Errorf("Failed to stop app: %v", err)
	}
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

	if bundle.stopped {
		t.Error("Bundle should not be stopped before Stop()")
	}

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := app.Stop(stopCtx); err != nil {
		t.Errorf("Failed to stop app: %v", err)
	}

	if !bundle.stopped {
		t.Error("Bundle should be stopped after Stop()")
	}
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
	if err := app.Stop(stopCtx); err != nil {
		t.Errorf("Failed to stop app: %v", err)
	}
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
	if err := app.Stop(stopCtx); err != nil {
		t.Errorf("Failed to stop app: %v", err)
	}
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
	if err := app.Stop(stopCtx); err != nil {
		t.Errorf("Failed to stop app: %v", err)
	}
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

// TestApp_Lifecycle_HTTPOnlyMode tests that an app with no gRPC registrars does not start a gRPC server.
func TestApp_Lifecycle_HTTPOnlyMode(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "http-only-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	component := &TestComponentWithCallbacks{}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(component),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// No gRPC registrars — grpcServer should remain nil after start
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start HTTP-only app: %v", err)
	}

	// gRPC server must not have been created
	if app.grpcServer != nil {
		t.Error("Expected grpcServer to be nil when no registrars are configured")
	}

	// gRPC health server must not have been created
	if app.grpcHealthServer != nil {
		t.Error("Expected grpcHealthServer to be nil when no registrars are configured")
	}

	// Component should still have started
	if !component.started {
		t.Error("Expected component to be started")
	}

	// HTTP server should be running (non-nil)
	if app.httpServer == nil {
		t.Error("Expected httpServer to be non-nil")
	}

	// Stop should succeed without error
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop HTTP-only app: %v", err)
	}

	if app.IsRunning() {
		t.Error("App should not be running after Stop()")
	}
}

// TestApp_Lifecycle_ComponentCallingIsRunningDuringStart verifies that a
// component calling IsRunning()/IsStopping() from its own Start() does not
// deadlock against App.Start() holding the app mutex across startup.
func TestApp_Lifecycle_ComponentCallingIsRunningDuringStart(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "is-running-during-start-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	comp := &isRunningCallingComponent{}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(comp),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	comp.app = app

	startErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		startErr <- app.Start(ctx)
	}()

	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Start returned an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start deadlocked when a component called IsRunning/IsStopping from its own Start")
	}

	if !comp.called {
		t.Fatal("expected component Start to have been called")
	}
	if comp.sawRunning {
		t.Error("expected IsRunning() to report false while the app is still starting")
	}
	if comp.sawStopping {
		t.Error("expected IsStopping() to report false during normal startup")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := app.Stop(stopCtx); err != nil {
		t.Errorf("Failed to stop app: %v", err)
	}
}

type isRunningCallingComponent struct {
	app         *App
	called      bool
	sawRunning  bool
	sawStopping bool
}

func (c *isRunningCallingComponent) Start(ctx context.Context) error {
	c.called = true
	c.sawRunning = c.app.IsRunning()
	c.sawStopping = c.app.IsStopping()
	return nil
}

func (c *isRunningCallingComponent) Stop(ctx context.Context) error {
	return nil
}

// TestApp_Lifecycle_ConcurrentStartRejected verifies that a second concurrent
// Start() call is rejected while the first is still in flight, rather than
// both proceeding or racing on the running/starting state.
func TestApp_Lifecycle_ConcurrentStartRejected(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "concurrent-start-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	slow := &TestComponentWithCallbacks{
		startFn: func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}

	app, err := New(
		WithConfig(&cfg),
		WithComponent(slow),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	const attempts = 2
	errs := make([]error, attempts)

	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			errs[idx] = app.Start(ctx)
		}(i)
	}
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case strings.Contains(err.Error(), "already running"):
			failures++
		default:
			t.Errorf("unexpected Start error: %v", err)
		}
	}

	if successes != 1 || failures != 1 {
		t.Fatalf("expected exactly one Start to succeed and one to be rejected, got %d successes and %d failures", successes, failures)
	}

	if !app.IsRunning() {
		t.Error("expected app to be running after the concurrent Start calls settle")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := app.Stop(stopCtx); err != nil {
		t.Errorf("Failed to stop app: %v", err)
	}
}

// TestApp_Lifecycle_ReadinessDelay_StopCancelsReadyFlip verifies that calling
// Stop() while a ReadinessInitialDelay is pending prevents the delayed
// SetReady(true) from firing after shutdown has begun, and that the delay
// goroutine exits promptly instead of leaking.
func TestApp_Lifecycle_ReadinessDelay_StopCancelsReadyFlip(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "readiness-delay-stop-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"
	cfg.ReadinessInitialDelay = 150 * time.Millisecond

	baseline := runtime.NumGoroutine()

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	if app.HealthRegistry().IsReady() {
		t.Fatal("expected service to not be ready immediately after Start with a readiness delay configured")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	if app.HealthRegistry().IsReady() {
		t.Error("expected service to not be ready immediately after Stop")
	}

	// Wait past the original delay to make sure the cancelled goroutine never
	// fires the delayed SetReady(true).
	time.Sleep(cfg.ReadinessInitialDelay * 3)

	if app.HealthRegistry().IsReady() {
		t.Error("expected readiness delay goroutine to not mark the service ready after Stop")
	}

	// Bounded wait for the goroutine population to settle back near baseline,
	// confirming the readiness delay goroutine (and everything else spun up
	// by Start/Stop) actually exited rather than leaking.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline+2 {
		if time.Now().After(deadline) {
			t.Errorf("goroutine count did not settle after Stop: baseline=%d, current=%d", baseline, runtime.NumGoroutine())
			break
		}
		time.Sleep(10 * time.Millisecond)
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
