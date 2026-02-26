// Package testutil provides testing utilities for the Forge framework.
//
// This package contains common testing patterns, mock implementations,
// and utility functions to make testing Forge applications easier and more reliable.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// TestApp wraps a Forge application for testing with automatic cleanup.
type TestApp struct {
	App     *framework.App
	Config  *config.BaseConfig
	cleanup func()
}

// NewTestApp creates a test application with default configuration and automatic cleanup.
func NewTestApp(t *testing.T, options ...framework.AppOption) *TestApp {
	t.Helper()

	// Create test configuration with random ports
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "test-service"
	cfg.GRPCAddr = getRandomPort()
	cfg.HTTPAddr = getRandomPort()
	cfg.ShutdownTimeout = 5 * time.Second

	// Add config to options
	allOptions := append([]framework.AppOption{framework.WithConfig(&cfg)}, options...)

	app, err := framework.New(allOptions...)
	if err != nil {
		t.Fatalf("Failed to create test app: %v", err)
	}

	testApp := &TestApp{
		App:    app,
		Config: &cfg,
		cleanup: func() {
			if app.IsRunning() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := app.Stop(ctx); err != nil {
					t.Errorf("Failed to stop test app: %v", err)
				}
			}
		},
	}

	// Register cleanup
	t.Cleanup(testApp.cleanup)

	return testApp
}

// Start starts the test application with a timeout.
func (ta *TestApp) Start(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ta.App.Start(ctx); err != nil {
		t.Fatalf("Failed to start test app: %v", err)
	}
}

// Stop stops the test application.
func (ta *TestApp) Stop(t *testing.T) {
	t.Helper()
	ta.cleanup()
}

// WaitForReady waits for the application to be ready.
func (ta *TestApp) WaitForReady(t *testing.T, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ta.App.IsRunning() {
			registry := ta.App.HealthRegistry()
			if registry != nil && registry.IsReady() {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatal("Application did not become ready within timeout")
}

// TestComponent provides a mock Component implementation for testing.
type TestComponent struct {
	Name        string
	StartCalled bool
	StopCalled  bool
	StartError  error
	StopError   error
	checks      []forgeHealth.Check
}

// NewTestComponent creates a new test component.
func NewTestComponent(name string) *TestComponent {
	return &TestComponent{
		Name: name,
		checks: []forgeHealth.Check{
			forgeHealth.NewAlwaysHealthyCheck(name),
		},
	}
}

// Start implements the Component interface.
func (c *TestComponent) Start(ctx context.Context) error {
	c.StartCalled = true
	return c.StartError
}

// Stop implements the Component interface.
func (c *TestComponent) Stop(ctx context.Context) error {
	c.StopCalled = true
	return c.StopError
}

// HealthChecks implements the HealthContributor interface.
func (c *TestComponent) HealthChecks() []forgeHealth.Check {
	return c.checks
}

// TestBundle provides a mock Bundle implementation for testing.
type TestBundle struct {
	BundleName   string
	InitCalled   bool
	InitError    error
	StopCalled   bool
	StopError    error
}

// NewTestBundle creates a new test bundle.
func NewTestBundle(name string) *TestBundle {
	return &TestBundle{
		BundleName: name,
	}
}

// Name implements the Bundle interface.
func (b *TestBundle) Name() string {
	return b.BundleName
}

// Initialize implements the Bundle interface.
func (b *TestBundle) Initialize(app *framework.App) error {
	b.InitCalled = true
	return b.InitError
}

// Stop implements the Bundle interface.
func (b *TestBundle) Stop(ctx context.Context) error {
	b.StopCalled = true
	return b.StopError
}

// getRandomPort returns a random available port for testing.
func getRandomPort() string {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return ":0" // Fallback to any port
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return fmt.Sprintf(":%d", addr.Port)
}

// TestHTTPClient provides HTTP client utilities for testing.
type TestHTTPClient struct {
	BaseURL string
	Client  *http.Client
}

// NewTestHTTPClient creates an HTTP client for testing Forge HTTP endpoints.
func NewTestHTTPClient(baseURL string) *TestHTTPClient {
	return &TestHTTPClient{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Get performs a GET request for testing.
func (c *TestHTTPClient) Get(t *testing.T, path string) *http.Response {
	t.Helper()

	resp, err := c.Client.Get(c.BaseURL + path)
	if err != nil {
		t.Fatalf("HTTP GET %s failed: %v", path, err)
	}

	return resp
}

// CheckHealth performs a health check and validates the response.
func (c *TestHTTPClient) CheckHealth(t *testing.T) {
	t.Helper()

	resp := c.Get(t, "/health")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected health check to return 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

// TestServer provides utilities for testing HTTP servers.
type TestServer struct {
	Server *httptest.Server
	URL    string
}

// NewTestServer creates a test HTTP server for testing.
func NewTestServer(handler http.Handler) *TestServer {
	server := httptest.NewServer(handler)
	return &TestServer{
		Server: server,
		URL:    server.URL,
	}
}

// Close closes the test server.
func (ts *TestServer) Close() {
	ts.Server.Close()
}

// AssertNoError is a test helper that fails the test if err is not nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

// AssertError is a test helper that fails the test if err is nil.
func AssertError(t *testing.T, err error, expectedMessage string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error with message '%s', got nil", expectedMessage)
	}
	if expectedMessage != "" && !strings.Contains(err.Error(), expectedMessage) {
		t.Fatalf("Expected error containing '%s', got: %v", expectedMessage, err)
	}
}

// AssertEqual is a test helper for equality assertions.
func AssertEqual[T comparable](t *testing.T, expected, actual T) {
	t.Helper()
	if expected != actual {
		t.Fatalf("Expected %v, got %v", expected, actual)
	}
}

// AssertTrue is a test helper for boolean assertions.
func AssertTrue(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Fatalf("Expected condition to be true: %s", message)
	}
}

// AssertFalse is a test helper for boolean assertions.
func AssertFalse(t *testing.T, condition bool, message string) {
	t.Helper()
	if condition {
		t.Fatalf("Expected condition to be false: %s", message)
	}
}

// NewLogger returns a zerolog.Logger suitable for testing (writes to os.Stderr).
func NewLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).With().Timestamp().Logger()
}

// Integration test helpers (require build tag: integration)

// SetupTestDB creates a test database connection for integration testing.
// Requires PostgreSQL to be running.
func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// This would connect to a test database
	// Implementation depends on testing infrastructure
	return nil
}

// SetupTestRedis creates a test Redis client for integration testing.
// Requires Redis to be running.
func SetupTestRedis(t *testing.T) redis.UniversalClient {
	t.Helper()

	// This would connect to a test Redis instance
	// Implementation depends on testing infrastructure
	return nil
}