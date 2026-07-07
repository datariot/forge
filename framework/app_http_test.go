package framework

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/datariot/forge/config"
	forgeHealth "github.com/datariot/forge/health"
)

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
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response["status"] == "healthy" {
		t.Error("Expected status to not be 'healthy' when service not ready")
	}
}

// TestApp_HealthEndpoints_RedactErrorDetail verifies that a failing
// dependency check's raw error (which can embed internal hostnames, IPs,
// and ports) never reaches the unauthenticated /health and /health/ready
// HTTP responses, while the overall unhealthy verdict is still reported.
func TestApp_HealthEndpoints_RedactErrorDetail(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "redact-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	const leakyErr = "dial tcp 10.0.1.5:5432: connect: connection refused"
	leaky := forgeHealth.NewAlwaysUnhealthyCheck("postgres", errors.New(leakyErr))
	if err := app.healthRegistry.Register(leaky, forgeHealth.DefaultCheckConfig("postgres")); err != nil {
		t.Fatalf("failed to register health check: %v", err)
	}

	cases := []struct {
		name    string
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"health", "/health", app.handleHealth},
		{"ready", "/health/ready", app.handleReady},
		{"live", "/health/live", app.handleLive},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			tc.handler(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("expected 503 for unhealthy required check, got %d", resp.StatusCode)
			}

			body := w.Body.String()
			if strings.Contains(body, "10.0.1.5:5432") {
				t.Errorf("expected response body to not leak raw dependency error, got: %s", body)
			}

			var parsed map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
				t.Fatalf("failed to parse JSON response: %v", err)
			}
			if parsed["status"] == "healthy" {
				t.Error("expected overall status to reflect the unhealthy required check")
			}
		})
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
	defer func() { _ = app.Stop(context.Background()) }()

	// Verify registrar was called
	if !registrarCalled {
		t.Error("Expected gRPC registrar to be called during startup")
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
