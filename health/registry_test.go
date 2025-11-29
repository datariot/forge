package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRegistry_Register tests basic check registration
func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry(nil)

	check := NewAlwaysHealthyCheck("test-check")
	config := DefaultCheckConfig("test-check")

	if err := registry.Register(check, config); err != nil {
		t.Errorf("Expected registration to succeed, got error: %v", err)
	}
}

// TestRegistry_Register_NilCheck tests registration fails with nil check
func TestRegistry_Register_NilCheck(t *testing.T) {
	registry := NewRegistry(nil)

	config := DefaultCheckConfig("test-check")

	if err := registry.Register(nil, config); err == nil {
		t.Error("Expected error when registering nil check")
	}
}

// TestRegistry_Register_EmptyName tests registration fails with empty name
func TestRegistry_Register_EmptyName(t *testing.T) {
	registry := NewRegistry(nil)

	check := &testCheck{name: ""}
	config := DefaultCheckConfig("")

	if err := registry.Register(check, config); err == nil {
		t.Error("Expected error when registering check with empty name")
	}
}

// TestRegistry_Register_Duplicate tests registration fails with duplicate name
func TestRegistry_Register_Duplicate(t *testing.T) {
	registry := NewRegistry(nil)

	check1 := NewAlwaysHealthyCheck("duplicate")
	check2 := NewAlwaysHealthyCheck("duplicate")
	config := DefaultCheckConfig("duplicate")

	if err := registry.Register(check1, config); err != nil {
		t.Fatalf("First registration should succeed: %v", err)
	}

	if err := registry.Register(check2, config); err == nil {
		t.Error("Expected error when registering duplicate check name")
	}
}

// TestRegistry_Unregister tests check unregistration
func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry(nil)

	check := NewAlwaysHealthyCheck("test-check")
	config := DefaultCheckConfig("test-check")

	registry.Register(check, config)
	registry.Unregister("test-check")

	// Should be able to re-register after unregister
	if err := registry.Register(check, config); err != nil {
		t.Errorf("Expected re-registration to succeed after unregister: %v", err)
	}
}

// TestRegistry_SetReady tests ready state management
func TestRegistry_SetReady(t *testing.T) {
	registry := NewRegistry(nil)

	if registry.IsReady() {
		t.Error("Registry should not be ready initially")
	}

	registry.SetReady(true)
	if !registry.IsReady() {
		t.Error("Registry should be ready after SetReady(true)")
	}

	registry.SetReady(false)
	if registry.IsReady() {
		t.Error("Registry should not be ready after SetReady(false)")
	}
}

// TestRegistry_CheckLiveness tests liveness check execution
func TestRegistry_CheckLiveness(t *testing.T) {
	registry := NewRegistry(nil)

	healthyCheck := NewAlwaysHealthyCheck("healthy-check")
	registry.Register(healthyCheck, DefaultCheckConfig("healthy-check"))

	ctx := context.Background()
	status := registry.CheckLiveness(ctx)

	if status.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, status.Status)
	}

	if !status.IsHealthy() {
		t.Error("Expected status to be healthy")
	}
}

// TestRegistry_CheckLiveness_FailingCheck tests liveness with failing check
func TestRegistry_CheckLiveness_FailingCheck(t *testing.T) {
	registry := NewRegistry(nil)

	failingCheck := &testCheck{
		name:          "failing-check",
		livenessErr:   errors.New("check failed"),
		readinessErr:  errors.New("check failed"),
	}
	config := DefaultCheckConfig("failing-check")
	config.Required = true

	registry.Register(failingCheck, config)

	ctx := context.Background()
	status := registry.CheckLiveness(ctx)

	if status.Status != StatusUnhealthy {
		t.Errorf("Expected status %s with failing required check, got %s", StatusUnhealthy, status.Status)
	}

	if status.IsHealthy() {
		t.Error("Expected status to be unhealthy with failing check")
	}
}

// TestRegistry_CheckLiveness_OptionalFailingCheck tests liveness with optional failing check
func TestRegistry_CheckLiveness_OptionalFailingCheck(t *testing.T) {
	registry := NewRegistry(nil)

	failingCheck := &testCheck{
		name:         "optional-failing",
		livenessErr:  errors.New("optional check failed"),
		readinessErr: errors.New("optional check failed"),
	}
	config := DefaultCheckConfig("optional-failing")
	config.Required = false

	registry.Register(failingCheck, config)

	ctx := context.Background()
	status := registry.CheckLiveness(ctx)

	// Optional check failure should still result in healthy overall status
	if status.Status != StatusHealthy {
		t.Errorf("Expected status %s with optional failing check, got %s", StatusHealthy, status.Status)
	}

	// But the check result should show it failed
	if checkResult, ok := status.Details["optional-failing"]; ok {
		if checkResult.IsHealthy() {
			t.Error("Expected optional check to show as failed in details")
		}
	}
}

// TestRegistry_CheckReadiness tests readiness check execution
func TestRegistry_CheckReadiness(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	healthyCheck := NewAlwaysHealthyCheck("healthy-check")
	registry.Register(healthyCheck, DefaultCheckConfig("healthy-check"))

	ctx := context.Background()
	status := registry.CheckReadiness(ctx)

	if status.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, status.Status)
	}

	if !status.IsReady() {
		t.Error("Expected status to be ready")
	}
}

// TestRegistry_CheckReadiness_NotMarkedReady tests readiness when not marked ready
func TestRegistry_CheckReadiness_NotMarkedReady(t *testing.T) {
	registry := NewRegistry(nil)
	// Don't call SetReady(true)

	healthyCheck := NewAlwaysHealthyCheck("healthy-check")
	registry.Register(healthyCheck, DefaultCheckConfig("healthy-check"))

	ctx := context.Background()
	status := registry.CheckReadiness(ctx)

	if status.Status != StatusUnhealthy {
		t.Errorf("Expected status %s when not marked ready, got %s", StatusUnhealthy, status.Status)
	}

	if status.IsReady() {
		t.Error("Expected status to not be ready when SetReady() not called")
	}
}

// TestRegistry_CheckHealth tests overall health check
func TestRegistry_CheckHealth(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	healthyCheck := NewAlwaysHealthyCheck("healthy-check")
	registry.Register(healthyCheck, DefaultCheckConfig("healthy-check"))

	ctx := context.Background()
	status := registry.CheckHealth(ctx)

	if status.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, status.Status)
	}
}

// TestRegistry_ConcurrentChecks tests concurrent check execution
func TestRegistry_ConcurrentChecks(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	// Add multiple checks with delays
	for i := 0; i < 5; i++ {
		check := &testCheck{
			name: "slow-check-" + string(rune('A'+i)),
			delay: 100 * time.Millisecond,
		}
		registry.Register(check, DefaultCheckConfig(check.name))
	}

	ctx := context.Background()
	start := time.Now()
	status := registry.CheckLiveness(ctx)
	duration := time.Since(start)

	// If checks run concurrently, total time should be ~100ms, not 500ms
	if duration > 300*time.Millisecond {
		t.Errorf("Expected concurrent execution (~100ms), took %v", duration)
	}

	if status.Status != StatusHealthy {
		t.Errorf("Expected all checks to pass, got status %s", status.Status)
	}

	// Should have 5 check results
	if len(status.Details) != 5 {
		t.Errorf("Expected 5 check results, got %d", len(status.Details))
	}
}

// TestRegistry_CheckTimeout tests timeout handling via context
func TestRegistry_CheckTimeout(t *testing.T) {
	registry := NewRegistry(nil)

	slowCheck := &testCheck{
		name:  "very-slow-check",
		delay: 5 * time.Second, // Longer than context timeout
	}
	config := DefaultCheckConfig("very-slow-check")
	config.Required = true

	registry.Register(slowCheck, config)

	// Use context timeout to control check duration
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_ = registry.CheckLiveness(ctx)
	duration := time.Since(start)

	// Note: Registry uses global context timeout, not per-check config.Timeout
	// This is a known limitation. Test verifies context timeout works.

	// Should timeout quickly via context, not wait 5 seconds
	if duration > 2*time.Second {
		t.Errorf("Expected context timeout to prevent long wait, took %v", duration)
	}
}

// TestRegistry_MixedChecks tests registry with mix of healthy and unhealthy checks
func TestRegistry_MixedChecks(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	// Required healthy check
	registry.Register(
		NewAlwaysHealthyCheck("required-healthy"),
		DefaultCheckConfig("required-healthy"),
	)

	// Optional failing check
	failingCheck := &testCheck{
		name:         "optional-failing",
		livenessErr:  errors.New("optional failure"),
		readinessErr: errors.New("optional failure"),
	}
	failingConfig := DefaultCheckConfig("optional-failing")
	failingConfig.Required = false
	registry.Register(failingCheck, failingConfig)

	ctx := context.Background()
	status := registry.CheckLiveness(ctx)

	// Should be healthy (required passes, optional failure doesn't affect overall status)
	if status.Status != StatusHealthy {
		t.Errorf("Expected status %s with mixed checks (required passes, optional fails), got %s", StatusHealthy, status.Status)
	}

	// Verify the optional check is recorded as failed in details
	if result, ok := status.Details["optional-failing"]; ok {
		if result.IsHealthy() {
			t.Error("Expected optional check to show as failed in details")
		}
		if result.Required {
			t.Error("Expected optional check to not be marked as required")
		}
	}
}

// Test helper: custom check with configurable behavior
type testCheck struct {
	name         string
	livenessErr  error
	readinessErr error
	delay        time.Duration
}

func (c *testCheck) Name() string {
	return c.name
}

func (c *testCheck) Liveness(ctx context.Context) error {
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.livenessErr
}

func (c *testCheck) Readiness(ctx context.Context) error {
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.readinessErr
}
