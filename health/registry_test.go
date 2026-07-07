package health

import (
	"context"
	"errors"
	"sync"
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

	if err := registry.Register(check, config); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}
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
	if err := registry.Register(healthyCheck, DefaultCheckConfig("healthy-check")); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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
		name:         "failing-check",
		livenessErr:  errors.New("check failed"),
		readinessErr: errors.New("check failed"),
	}
	config := DefaultCheckConfig("failing-check")
	config.Required = true

	if err := registry.Register(failingCheck, config); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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

	if err := registry.Register(failingCheck, config); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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
	if err := registry.Register(healthyCheck, DefaultCheckConfig("healthy-check")); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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
	if err := registry.Register(healthyCheck, DefaultCheckConfig("healthy-check")); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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
	if err := registry.Register(healthyCheck, DefaultCheckConfig("healthy-check")); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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
			name:  "slow-check-" + string(rune('A'+i)),
			delay: 100 * time.Millisecond,
		}
		if err := registry.Register(check, DefaultCheckConfig(check.name)); err != nil {
			t.Fatalf("Failed to register check: %v", err)
		}
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

	if err := registry.Register(slowCheck, config); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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
	if err := registry.Register(
		NewAlwaysHealthyCheck("required-healthy"),
		DefaultCheckConfig("required-healthy"),
	); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

	// Optional failing check
	failingCheck := &testCheck{
		name:         "optional-failing",
		livenessErr:  errors.New("optional failure"),
		readinessErr: errors.New("optional failure"),
	}
	failingConfig := DefaultCheckConfig("optional-failing")
	failingConfig.Required = false
	if err := registry.Register(failingCheck, failingConfig); err != nil {
		t.Fatalf("Failed to register check: %v", err)
	}

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

func TestRegistry_MustRegister_Success(t *testing.T) {
	registry := NewRegistry(nil)
	check := NewAlwaysHealthyCheck("must-check")
	cfg := DefaultCheckConfig("must-check")
	// Should not panic
	registry.MustRegister(check, cfg)
}

func TestRegistry_MustRegister_Panic(t *testing.T) {
	registry := NewRegistry(nil)
	check := NewAlwaysHealthyCheck("must-check")
	cfg := DefaultCheckConfig("must-check")
	registry.MustRegister(check, cfg)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate MustRegister")
		}
	}()
	registry.MustRegister(check, cfg)
}

func TestRegistry_RegisteredChecks(t *testing.T) {
	registry := NewRegistry(nil)
	check1 := NewAlwaysHealthyCheck("check-1")
	check2 := NewAlwaysHealthyCheck("check-2")
	registry.MustRegister(check1, DefaultCheckConfig("check-1"))
	registry.MustRegister(check2, DefaultCheckConfig("check-2"))

	names := registry.RegisteredChecks()
	if len(names) != 2 {
		t.Fatalf("expected 2 registered checks, got %d", len(names))
	}
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["check-1"] || !found["check-2"] {
		t.Errorf("expected check-1 and check-2, got %v", names)
	}
}

func TestRegistry_CheckConfig(t *testing.T) {
	registry := NewRegistry(nil)
	cfg := DefaultCheckConfig("db")
	cfg.Required = false
	registry.MustRegister(NewAlwaysHealthyCheck("db"), cfg)

	got, ok := registry.CheckConfig("db")
	if !ok {
		t.Fatal("expected CheckConfig to return true for registered check")
	}
	if got.Name != "db" {
		t.Errorf("expected config name 'db', got %q", got.Name)
	}
	if got.Required {
		t.Error("expected Required=false")
	}

	_, ok2 := registry.CheckConfig("nonexistent")
	if ok2 {
		t.Error("expected CheckConfig to return false for unknown check")
	}
}

func TestNoopLogger_AllMethods(t *testing.T) {
	var l NoopLogger
	l.Debug("debug", "k", "v")
	l.Info("info", "k", "v")
	l.Warn("warn", "k", "v")
	l.Error("error", "k", "v")
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

// blockingCheck blocks Liveness/Readiness on ctx.Done() (never returning on
// its own), letting a test drive it via context cancellation only. onCall,
// if set, is invoked (once, safely from concurrent probes) the first time
// either method is entered.
type blockingCheck struct {
	name string

	once   sync.Once
	onCall func()
}

func (c *blockingCheck) Name() string { return c.name }

func (c *blockingCheck) block(ctx context.Context) error {
	if c.onCall != nil {
		c.once.Do(c.onCall)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *blockingCheck) Liveness(ctx context.Context) error  { return c.block(ctx) }
func (c *blockingCheck) Readiness(ctx context.Context) error { return c.block(ctx) }

// gatedCheck succeeds immediately for its first roundSize invocations (one
// full round's worth of Liveness+Readiness calls), then blocks on ctx.Done()
// for every invocation after that. It's used to simulate a dependency that
// is healthy on the first background round and then hangs on the next one.
type gatedCheck struct {
	name      string
	roundSize int

	mu    sync.Mutex
	calls int

	firstRoundDone chan struct{}
	once           sync.Once
}

func newGatedCheck(name string) *gatedCheck {
	return &gatedCheck{
		name:           name,
		roundSize:      2, // liveness + readiness
		firstRoundDone: make(chan struct{}),
	}
}

func (c *gatedCheck) Name() string { return c.name }

func (c *gatedCheck) probe(ctx context.Context) error {
	c.mu.Lock()
	c.calls++
	n := c.calls
	c.mu.Unlock()

	if n <= c.roundSize {
		if n == c.roundSize {
			c.once.Do(func() { close(c.firstRoundDone) })
		}
		return nil
	}

	<-ctx.Done()
	return ctx.Err()
}

func (c *gatedCheck) Liveness(ctx context.Context) error  { return c.probe(ctx) }
func (c *gatedCheck) Readiness(ctx context.Context) error { return c.probe(ctx) }

// waitClosed waits for ch to be closed, failing the test if it isn't within d.
func waitClosed(t *testing.T, ch chan struct{}, d time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// TestRegistry_Runner_CachesResultsAndServesStaleReads verifies that once
// Start has been called, a probe read is served from the background
// runner's cache: it returns near-instantly and reflects the last
// completed round, even while a new round is in flight (e.g. blocked on a
// hanging dependency).
func TestRegistry_Runner_CachesResultsAndServesStaleReads(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	check := newGatedCheck("dep")
	cfg := DefaultCheckConfig("dep")
	cfg.Interval = 10 * time.Millisecond
	cfg.InitialDelay = 0
	cfg.Timeout = time.Second
	if err := registry.Register(check, cfg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := registry.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		if err := registry.Stop(stopCtx); err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	})

	// Wait for the first round to complete so the cache is populated.
	waitClosed(t, check.firstRoundDone, 2*time.Second, "first background round")

	// The second round is now in flight and will block forever (until
	// ctx/Stop cancellation). A probe read must not wait for it - it
	// should return the still-healthy result from the first round.
	done := make(chan Report, 1)
	go func() { done <- registry.CheckReadiness(context.Background()) }()

	select {
	case status := <-done:
		if status.Status != StatusHealthy {
			t.Errorf("expected cached status healthy, got %s (details: %+v)", status.Status, status.Details)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("CheckReadiness blocked on the in-flight (hanging) check instead of serving the cache")
	}
}

// TestRegistry_Runner_PreFirstRoundReadinessNotReady verifies that a probe
// hitting the registry before the background runner's first round has
// completed reports not-ready, rather than optimistically healthy.
func TestRegistry_Runner_PreFirstRoundReadinessNotReady(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	check := &blockingCheck{name: "slow-start"}
	cfg := DefaultCheckConfig("slow-start")
	cfg.InitialDelay = time.Hour // effectively "never during this test"
	if err := registry.Register(check, cfg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := registry.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		registry.Stop(stopCtx) //nolint:errcheck
	})

	// InitialDelay means the runner goroutine hasn't run a round yet, so
	// this must be answered from the "pending" synthetic result, not by
	// invoking the (permanently blocking) check.
	status := registry.CheckReadiness(context.Background())
	if status.Status != StatusUnhealthy {
		t.Errorf("expected pending check to report unhealthy/not-ready, got %s", status.Status)
	}
	if result, ok := status.Details["slow-start"]; ok {
		if result.Status != StatusUnknown {
			t.Errorf("expected pending check detail status %s, got %s", StatusUnknown, result.Status)
		}
	} else {
		t.Error("expected a details entry for the pending check")
	}
}

// TestRegistry_Runner_LateRegisteredCheckGetsScheduled verifies that a
// check registered after Start still gets its own background runner.
func TestRegistry_Runner_LateRegisteredCheckGetsScheduled(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := registry.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		if err := registry.Stop(stopCtx); err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	})

	check := newGatedCheck("late")
	cfg := DefaultCheckConfig("late")
	cfg.Interval = 10 * time.Millisecond
	if err := registry.Register(check, cfg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	waitClosed(t, check.firstRoundDone, 2*time.Second, "late-registered check's first background round")

	status := registry.CheckLiveness(context.Background())
	if status.Status != StatusHealthy {
		t.Errorf("expected late-registered check to report healthy after its round, got %s", status.Status)
	}
}

// TestRegistry_Stop_NoGoroutineLeak verifies that Stop terminates all
// runner goroutines - including one currently blocked inside a check -
// without leaking. The assertion is that Stop itself returns (its
// implementation waits on a done channel derived from the runner
// goroutines' WaitGroup), not a fixed sleep.
func TestRegistry_Stop_NoGoroutineLeak(t *testing.T) {
	registry := NewRegistry(nil)

	started := make(chan struct{})
	check := &blockingCheck{onCall: func() { close(started) }, name: "blocking"}
	cfg := DefaultCheckConfig("blocking")
	if err := registry.Register(check, cfg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	waitClosed(t, started, 2*time.Second, "background runner to invoke the blocking check")

	stopDone := make(chan error, 1)
	go func() { stopDone <- registry.Stop(context.Background()) }()

	select {
	case err := <-stopDone:
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return in time - a runner goroutine leaked")
	}
}

// TestRegistry_Fallback_NotStarted verifies the registry still computes
// live when Start has never been called, preserving the pre-existing API
// contract for unit tests / direct library use.
func TestRegistry_Fallback_NotStarted(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SetReady(true)

	calls := 0
	var mu sync.Mutex
	check := &testCheck{name: "fallback"}
	// Wrap to count invocations without changing testCheck's shape.
	counting := &countingCheck{testCheck: check, onCall: func() {
		mu.Lock()
		calls++
		mu.Unlock()
	}}

	if err := registry.Register(counting, DefaultCheckConfig("fallback")); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		status := registry.CheckLiveness(ctx)
		if status.Status != StatusHealthy {
			t.Fatalf("expected healthy status, got %s", status.Status)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Errorf("expected the fallback (non-started) path to invoke the check live on every call (3), got %d", calls)
	}
}

// countingCheck wraps a testCheck and calls onCall on every Liveness call,
// used to distinguish "computed live" from "served from cache".
type countingCheck struct {
	*testCheck
	onCall func()
}

func (c *countingCheck) Liveness(ctx context.Context) error {
	if c.onCall != nil {
		c.onCall()
	}
	return c.testCheck.Liveness(ctx)
}
