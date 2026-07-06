package testutil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewTestComponent(t *testing.T) {
	comp := NewTestComponent("my-component")
	if comp.Name != "my-component" {
		t.Errorf("expected name 'my-component', got %q", comp.Name)
	}
	if comp.StartCalled {
		t.Error("StartCalled should be false initially")
	}
	if comp.StopCalled {
		t.Error("StopCalled should be false initially")
	}
	checks := comp.HealthChecks()
	if len(checks) != 1 {
		t.Fatalf("expected 1 health check, got %d", len(checks))
	}
	if checks[0].Name() != "my-component" {
		t.Errorf("expected check name 'my-component', got %q", checks[0].Name())
	}
}

func TestTestComponent_StartStop(t *testing.T) {
	comp := NewTestComponent("test")

	if err := comp.Start(context.TODO()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !comp.StartCalled {
		t.Error("StartCalled should be true after Start()")
	}

	if err := comp.Stop(context.TODO()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !comp.StopCalled {
		t.Error("StopCalled should be true after Stop()")
	}
}

func TestTestComponent_StartError(t *testing.T) {
	comp := NewTestComponent("test")
	comp.StartError = errors.New("start failed")

	if err := comp.Start(context.TODO()); err == nil {
		t.Error("expected error from Start(), got nil")
	}
}

func TestTestComponent_StopError(t *testing.T) {
	comp := NewTestComponent("test")
	comp.StopError = errors.New("stop failed")

	if err := comp.Stop(context.TODO()); err == nil {
		t.Error("expected error from Stop(), got nil")
	}
}

func TestNewTestBundle(t *testing.T) {
	bundle := NewTestBundle("my-bundle")
	if bundle.Name() != "my-bundle" {
		t.Errorf("expected name 'my-bundle', got %q", bundle.Name())
	}
	if bundle.InitCalled {
		t.Error("InitCalled should be false initially")
	}
}

func TestTestBundle_InitializeStop(t *testing.T) {
	bundle := NewTestBundle("test")

	if err := bundle.Initialize(nil); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !bundle.InitCalled {
		t.Error("InitCalled should be true after Initialize()")
	}

	if err := bundle.Stop(context.TODO()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !bundle.StopCalled {
		t.Error("StopCalled should be true after Stop()")
	}
}

func TestTestBundle_InitError(t *testing.T) {
	bundle := NewTestBundle("test")
	bundle.InitError = errors.New("init failed")

	if err := bundle.Initialize(nil); err == nil {
		t.Error("expected error from Initialize(), got nil")
	}
}

func TestAssertNoError(t *testing.T) {
	// Should not fail the test
	AssertNoError(t, nil)
}

func TestAssertError(t *testing.T) {
	err := errors.New("something bad happened")
	AssertError(t, err, "something bad")
}

func TestAssertError_AnyMessage(t *testing.T) {
	err := errors.New("any error")
	// Empty expectedMessage means any error passes
	AssertError(t, err, "")
}

func TestAssertEqual(t *testing.T) {
	AssertEqual(t, 42, 42)
	AssertEqual(t, "hello", "hello")
	AssertEqual(t, true, true)
}

func TestAssertTrue(t *testing.T) {
	AssertTrue(t, true, "should be true")
}

func TestAssertFalse(t *testing.T) {
	AssertFalse(t, false, "should be false")
}

func TestNewTestHTTPClient(t *testing.T) {
	client := NewTestHTTPClient("http://localhost:8080")
	if client.BaseURL != "http://localhost:8080" {
		t.Errorf("expected base URL 'http://localhost:8080', got %q", client.BaseURL)
	}
	if client.Client == nil {
		t.Error("expected non-nil HTTP client")
	}
}

func TestNewTestServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := NewTestServer(handler)
	defer server.Close()

	if server.URL == "" {
		t.Error("expected non-empty server URL")
	}
	if server.Server == nil {
		t.Error("expected non-nil server")
	}
}

func TestTestHTTPClient_Get(t *testing.T) {
	// Set up test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	client := NewTestHTTPClient(testServer.URL)
	resp := client.Get(t, "/")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTestHTTPClient_CheckHealth(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer testServer.Close()

	client := NewTestHTTPClient(testServer.URL)
	client.CheckHealth(t)
}

func TestNewTestApp(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		t.Fatal("expected non-nil test app")
	}
	if app.App == nil {
		t.Error("expected non-nil app")
	}
	if app.Config == nil {
		t.Error("expected non-nil config")
	}
}

func TestTestApp_StartStop(t *testing.T) {
	app := NewTestApp(t)
	app.Start(t)

	if !app.App.IsRunning() {
		t.Error("expected app to be running after Start()")
	}

	app.Stop(t)
}
