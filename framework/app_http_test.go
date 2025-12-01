package framework

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/datariot/forge/config"
	forgeHealth "github.com/datariot/forge/health"
)

// TestApp_HandleHealth tests the /health endpoint
func TestApp_HandleHealth(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "health-endpoint-test"
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
	defer app.Stop(context.Background())

	// Create test request
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// Call handler
	app.handleHealth(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	// Verify JSON response
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}
}

// TestApp_HandleReady tests the /health/ready endpoint
func TestApp_HandleReady(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "ready-endpoint-test"
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
		t.Fatalf("Failed to start app: %v", err)
	}
	defer app.Stop(context.Background())

	// Create test request
	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()

	// Call handler
	app.handleReady(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	// Verify JSON response
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}
}

// TestApp_HandleReady_NotReady tests readiness when service not marked ready
func TestApp_HandleReady_NotReady(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "not-ready-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(
		WithConfig(&cfg),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Don't start app - service won't be ready

	// Create test request
	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()

	// Call handler directly without starting
	app.handleReady(w, req)

	// Check response - should be unhealthy/503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	// Verify JSON response
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response["status"] == "healthy" {
		t.Error("Expected status to not be 'healthy' when service not ready")
	}
}

// TestApp_HandleLive tests the /health/live endpoint
func TestApp_HandleLive(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "live-endpoint-test"
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
		t.Fatalf("Failed to start app: %v", err)
	}
	defer app.Stop(context.Background())

	// Create test request
	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	// Call handler
	app.handleLive(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	// Verify JSON response
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}
}

// TestApp_GetterMethods tests Config(), Logger(), HealthRegistry() getters
func TestApp_GetterMethods(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "getter-test"

	app, err := New(
		WithConfig(&cfg),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Test Config getter
	if app.Config() == nil {
		t.Error("Expected Config() to return non-nil")
	}

	if app.Config().ServiceName != "getter-test" {
		t.Errorf("Expected service name 'getter-test', got %s", app.Config().ServiceName)
	}

	// Test Logger getter
	if app.Logger() == nil {
		t.Error("Expected Logger() to return non-nil")
	}

	// Test HealthRegistry getter
	if app.HealthRegistry() == nil {
		t.Error("Expected HealthRegistry() to return non-nil")
	}
}

// TestApp_WithGRPCRegistrar tests gRPC service registration
func TestApp_WithGRPCRegistrar(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "grpc-registrar-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	registrarCalled := false
	registrar := &testRegistrar{
		registerFn: func() error {
			registrarCalled = true
			return nil
		},
	}

	app, err := New(
		WithConfig(&cfg),
		WithGRPCRegistrar(registrar),
	)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}
	defer app.Stop(context.Background())

	// Verify registrar was called
	if !registrarCalled {
		t.Error("Expected gRPC registrar to be called during startup")
	}
}

// TestApp_WithGRPCRegistrar_Nil tests nil registrar validation
func TestApp_WithGRPCRegistrar_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()

	_, err := New(
		WithConfig(&cfg),
		WithGRPCRegistrar(nil),
	)

	if err == nil {
		t.Error("Expected error for nil registrar")
	}
}

// Helper test types

type testRegistrar struct {
	registerFn func() error
}

func (r *testRegistrar) RegisterGRPC(server *grpc.Server) error {
	if r.registerFn != nil {
		return r.registerFn()
	}
	return nil
}
