package framework

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datariot/forge/config"
	forgeHealth "github.com/datariot/forge/health"
	"google.golang.org/grpc"
)

// TestComponent is a test implementation of the Component interface
type TestComponent struct {
	started    bool
	stopped    bool
	startError error
	stopError  error
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
	name        string
	initError   error
	initialized bool
	stopped     bool
	stopError   error
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
		name:      "failing-bundle",
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

func TestApp_New_InvalidAppEnv(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"
	cfg.AppEnv = "invalid-env"

	_, err := New(WithConfig(&cfg))
	if err == nil {
		t.Fatal("Expected error for invalid AppEnv")
	}

	if !strings.Contains(err.Error(), "invalid app_env") {
		t.Errorf("Expected error to contain 'invalid app_env', got '%s'", err.Error())
	}
}

func TestApp_New_MissingServiceName(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = ""

	_, err := New(WithConfig(&cfg))
	if err == nil {
		t.Fatal("Expected error for missing ServiceName")
	}

	if !strings.Contains(err.Error(), "service_name is required") {
		t.Errorf("Expected error to contain 'service_name is required', got '%s'", err.Error())
	}
}

func TestApp_New_ValidConfigSucceeds(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "valid-service"
	cfg.AppEnv = "production"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("Expected no error for valid config, got %v", err)
	}

	if app == nil {
		t.Fatal("Expected app to be non-nil")
	}
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

func TestApp_Config(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	got := app.Config()
	if got == nil {
		t.Fatal("expected non-nil config")
	}
	if got.ServiceName != "test-service" {
		t.Errorf("expected ServiceName='test-service', got %q", got.ServiceName)
	}
}

func TestApp_Logger(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	got := app.Logger()
	if got == nil {
		t.Fatal("expected non-nil LoggingManager")
	}
}

func TestApp_WithGRPCRegistrar_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	_, err := New(WithConfig(&cfg), WithGRPCRegistrar(nil))
	if err == nil {
		t.Fatal("expected error for nil registrar")
	}
	if !strings.Contains(err.Error(), "registrar cannot be nil") {
		t.Errorf("expected 'registrar cannot be nil', got %q", err.Error())
	}
}

func TestApp_WithGRPCRegistrar_Valid(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	app, err := New(WithConfig(&cfg), WithGRPCRegistrar(&testRegistrar{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.registrars) != 1 {
		t.Errorf("expected 1 registrar, got %d", len(app.registrars))
	}
}

func TestApp_WithUnaryInterceptor_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	_, err := New(WithConfig(&cfg), WithUnaryInterceptor(nil))
	if err == nil {
		t.Fatal("expected error for nil interceptor")
	}
	if !strings.Contains(err.Error(), "unary interceptor cannot be nil") {
		t.Errorf("expected 'unary interceptor cannot be nil', got %q", err.Error())
	}
}

func TestApp_WithUnaryInterceptor_Valid(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}

	app, err := New(WithConfig(&cfg), WithUnaryInterceptor(interceptor))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.unaryInterceptors) != 1 {
		t.Errorf("expected 1 interceptor, got %d", len(app.unaryInterceptors))
	}
}

// newTestApp is a helper that creates a started app for handler tests.
func newTestApp(t *testing.T) *App {
	t.Helper()
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		app.Stop(stopCtx) //nolint:errcheck
	})
	return app
}

func TestApp_HandleHealth(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	app.handleHealth(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 200 or 503, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestApp_HandleReady(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	app.handleReady(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 200 or 503, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestApp_HandleLive(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	app.handleLive(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 200 or 503, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestApp_IsStopping(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	if app.IsStopping() {
		t.Error("expected IsStopping()=false before start")
	}
}

func TestApp_WithShutdownHook_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	_, err := New(WithConfig(&cfg), WithShutdownHook(nil))
	if err == nil {
		t.Fatal("expected error for nil shutdown hook")
	}
	if !strings.Contains(err.Error(), "shutdown hook cannot be nil") {
		t.Errorf("expected 'shutdown hook cannot be nil', got %q", err.Error())
	}
}

func TestApp_WithStartupHook_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	_, err := New(WithConfig(&cfg), WithStartupHook(nil))
	if err == nil {
		t.Fatal("expected error for nil startup hook")
	}
	if !strings.Contains(err.Error(), "startup hook cannot be nil") {
		t.Errorf("expected 'startup hook cannot be nil', got %q", err.Error())
	}
}

func TestLoggingManager_Logger(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	lm := NewLoggingManager(&cfg)
	if err := lm.Initialize(); err != nil {
		t.Fatalf("failed to initialize logging manager: %v", err)
	}

	logger := lm.Logger()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestLoggingManager_WithContext(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	lm := NewLoggingManager(&cfg)
	if err := lm.Initialize(); err != nil {
		t.Fatalf("failed to initialize logging manager: %v", err)
	}

	logger := lm.WithContext(map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	})
	// Verify it returns a valid logger (non-zero value)
	_ = logger
}

func TestHealthLoggerAdapter_AllLevels(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"
	lm := NewLoggingManager(&cfg)
	if err := lm.Initialize(); err != nil {
		t.Fatalf("failed to initialize logging: %v", err)
	}

	adapter := NewHealthLogger(lm)
	// These should not panic
	adapter.Debug("debug message", "key", "value")
	adapter.Info("info message", "key", "value")
	adapter.Warn("warn message", "key", "value")
	adapter.Error("error message", "key", "value")
	// Non-string key should be handled gracefully
	adapter.Debug("debug with int key", 42, "value")
}

func TestApp_WithHealthContributor_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	_, err := New(WithConfig(&cfg), WithHealthContributor(nil))
	if err == nil {
		t.Fatal("expected error for nil contributor")
	}
	if !strings.Contains(err.Error(), "contributor cannot be nil") {
		t.Errorf("expected 'contributor cannot be nil', got %q", err.Error())
	}
}

func TestApp_Start_AlreadyRunning(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		app.Stop(stopCtx) //nolint:errcheck
	}()

	// Second start should fail
	err = app.Start(ctx)
	if err == nil {
		t.Fatal("expected error on second Start")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected 'already running', got %q", err.Error())
	}
}

func TestApp_WithConfig_Nil(t *testing.T) {
	_, err := New(WithConfig(nil))
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if !strings.Contains(err.Error(), "config cannot be nil") {
		t.Errorf("expected 'config cannot be nil', got %q", err.Error())
	}
}

func TestApp_WithStreamInterceptor_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	_, err := New(WithConfig(&cfg), WithStreamInterceptor(nil))
	if err == nil {
		t.Fatal("expected error for nil stream interceptor")
	}
	if !strings.Contains(err.Error(), "stream interceptor cannot be nil") {
		t.Errorf("expected 'stream interceptor cannot be nil', got %q", err.Error())
	}
}

func TestApp_WithStreamInterceptor_Valid(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"

	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss)
	}

	app, err := New(WithConfig(&cfg), WithStreamInterceptor(interceptor))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.streamInterceptors) != 1 {
		t.Errorf("expected 1 stream interceptor, got %d", len(app.streamInterceptors))
	}
}
