package health

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"
)

// defaultCheckTimeout is used for a single check invocation when the
// registered CheckConfig does not specify a Timeout.
const defaultCheckTimeout = 5 * time.Second

// defaultCheckInterval is used for the background runner when the
// registered CheckConfig does not specify an Interval.
const defaultCheckInterval = 30 * time.Second

// probeKind selects which side of a Check (liveness or readiness) to invoke.
// It carries a method-value extractor rather than a string tag so that
// dispatch has no "unknown kind" branch to fall through to - only the two
// package-level values below exist, and each knows how to run itself.
// Callers always pass one of *probeLiveness / *probeReadiness, which makes
// pointer identity (used in snapshotStatus) a valid way to tell them apart.
type probeKind struct {
	label string
	fn    func(Check) func(context.Context) error
}

// probeLiveness dispatches to Check.Liveness.
var probeLiveness = &probeKind{
	label: "liveness",
	fn:    func(c Check) func(context.Context) error { return c.Liveness },
}

// probeReadiness dispatches to Check.Readiness.
var probeReadiness = &probeKind{
	label: "readiness",
	fn:    func(c Check) func(context.Context) error { return c.Readiness },
}

// checkCache holds the most recent background results for a single
// registered check. A zero-value checkCache (ran == false) means the
// background runner has not completed a round for this check yet.
type checkCache struct {
	liveness  CheckResult
	readiness CheckResult
	ran       bool
}

// Registry manages a collection of health checks and provides methods
// to check the overall health status of a service.
//
// By default, Registry computes results live on every call (the original
// behavior, and what unit tests and direct library use still get). Calling
// Start runs checks in the background on their configured Interval and
// serves cached results instead, so a probe read never blocks on a slow or
// hanging dependency check.
type Registry struct {
	mu      sync.RWMutex
	checks  map[string]Check
	configs map[string]CheckConfig
	ready   bool
	logger  Logger

	// Background runner state, guarded by mu.
	started   bool
	runnerCtx context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	cache     map[string]*checkCache
}

// Logger interface for health check logging.
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NoopLogger is a logger that does nothing.
type NoopLogger struct{}

func (NoopLogger) Debug(msg string, fields ...interface{}) {}
func (NoopLogger) Info(msg string, fields ...interface{})  {}
func (NoopLogger) Warn(msg string, fields ...interface{})  {}
func (NoopLogger) Error(msg string, fields ...interface{}) {}

// NewRegistry creates a new health check registry.
func NewRegistry(logger Logger) *Registry {
	if logger == nil {
		logger = NoopLogger{}
	}

	return &Registry{
		checks:  make(map[string]Check),
		configs: make(map[string]CheckConfig),
		ready:   false,
		logger:  logger,
		cache:   make(map[string]*checkCache),
	}
}

// Register registers a health check with the registry.
//
// If the background runner is already started (see Start), the new check
// is picked up immediately and scheduled on its own goroutine so that
// checks registered after startup still get background execution.
func (r *Registry) Register(check Check, config CheckConfig) error {
	if check == nil {
		return fmt.Errorf("check cannot be nil")
	}

	name := check.Name()
	if name == "" {
		return fmt.Errorf("check name cannot be empty")
	}

	r.mu.Lock()

	if _, exists := r.checks[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("check with name %q already registered", name)
	}

	r.checks[name] = check
	r.configs[name] = config
	r.cache[name] = &checkCache{}

	started := r.started
	runnerCtx := r.runnerCtx
	if started {
		r.wg.Add(1)
	}
	r.mu.Unlock()

	r.logger.Info("registered health check", "name", name, "required", config.Required)

	if started {
		go r.runCheck(runnerCtx, name, check, config)
	}

	return nil
}

// MustRegister registers a health check and panics if registration fails.
func (r *Registry) MustRegister(check Check, config CheckConfig) {
	if err := r.Register(check, config); err != nil {
		panic(fmt.Sprintf("failed to register health check: %v", err))
	}
}

// Unregister removes a health check from the registry.
//
// If the background runner is active, the check's goroutine notices the
// removal on its next tick (or initial round) and exits on its own; there
// is nothing further to clean up here.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.checks, name)
	delete(r.configs, name)
	delete(r.cache, name)

	r.logger.Info("unregistered health check", "name", name)
}

// SetReady marks the service as ready. This affects the overall readiness status.
func (r *Registry) SetReady(ready bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ready = ready
	r.logger.Info("service readiness changed", "ready", ready)
}

// IsReady returns true if the service has been explicitly marked as ready.
func (r *Registry) IsReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

// Start launches the background check runner: one goroutine per registered
// check that waits its CheckConfig.InitialDelay, runs an immediate round,
// and then re-runs on CheckConfig.Interval until Stop is called or ctx is
// done. Once started, CheckLiveness/CheckReadiness/CheckHealth serve the
// cached results from these rounds instead of computing live, so a probe
// read is never blocked by a slow or hanging dependency.
//
// Start is not safe to call twice without an intervening Stop.
func (r *Registry) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return fmt.Errorf("health registry runner already started")
	}

	runnerCtx, cancel := context.WithCancel(ctx)
	r.runnerCtx = runnerCtx
	r.cancel = cancel
	r.started = true

	checks := maps.Clone(r.checks)
	configs := maps.Clone(r.configs)
	r.mu.Unlock()

	for name, check := range checks {
		r.wg.Add(1)
		go r.runCheck(runnerCtx, name, check, configs[name])
	}

	return nil
}

// Stop cancels all background runner goroutines and waits for them to
// exit, or for ctx to be done, whichever comes first. It is safe to call
// even if Start was never called. After Stop returns (with a nil error),
// no runner goroutines remain.
func (r *Registry) Stop(ctx context.Context) error {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return nil
	}

	cancel := r.cancel
	r.started = false
	r.cancel = nil
	r.runnerCtx = nil
	r.mu.Unlock()

	cancel()

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runCheck is the background goroutine body for a single check: wait for
// InitialDelay, run an immediate round, then run one round per Interval
// until ctx is cancelled or the check is unregistered.
func (r *Registry) runCheck(ctx context.Context, name string, check Check, config CheckConfig) {
	defer r.wg.Done()

	if config.InitialDelay > 0 {
		select {
		case <-time.After(config.InitialDelay):
		case <-ctx.Done():
			return
		}
	}

	r.runRound(ctx, name, check, config)

	interval := config.Interval
	if interval <= 0 {
		interval = defaultCheckInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !r.stillRegistered(name) {
				return
			}
			r.runRound(ctx, name, check, config)
		}
	}
}

// stillRegistered reports whether name is still a registered check.
func (r *Registry) stillRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.checks[name]
	return ok
}

// runRound executes one liveness+readiness round for a single check and
// stores the results in the cache under mu.
func (r *Registry) runRound(ctx context.Context, name string, check Check, config CheckConfig) {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}

	liveness := r.executeOne(ctx, timeout, name, check, config, probeLiveness)
	readiness := r.executeOne(ctx, timeout, name, check, config, probeReadiness)

	r.mu.Lock()
	entry, ok := r.cache[name]
	if !ok {
		entry = &checkCache{}
		r.cache[name] = entry
	}
	entry.liveness = liveness
	entry.readiness = readiness
	entry.ran = true
	r.mu.Unlock()
}

// executeOne runs a single probe against a single check with a per-run
// timeout derived from parent, and returns the resulting CheckResult.
func (r *Registry) executeOne(parent context.Context, timeout time.Duration, name string, check Check, config CheckConfig, kind *probeKind) CheckResult {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	start := time.Now()
	err := kind.fn(check)(ctx)
	duration := time.Since(start)

	result := NewCheckResult(name, config.Required, duration, err)

	if err != nil {
		r.logger.Warn("health check failed", "name", name, "type", kind.label, "error", err, "duration", duration)
	} else {
		r.logger.Debug("health check passed", "name", name, "type", kind.label, "duration", duration)
	}

	return result
}

// CheckLiveness performs liveness checks on all registered health checks.
//
// If the background runner is active, this serves the latest cached
// results instead of computing them live.
func (r *Registry) CheckLiveness(ctx context.Context) HealthStatus {
	r.mu.RLock()
	if r.started {
		status := r.snapshotStatus(probeLiveness)
		r.mu.RUnlock()
		return status
	}
	checks := maps.Clone(r.checks)
	configs := maps.Clone(r.configs)
	r.mu.RUnlock()

	// Add overall timeout for all liveness checks
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return r.performChecks(ctx, checks, configs, probeLiveness)
}

// CheckReadiness performs readiness checks on all registered health checks.
//
// If the background runner is active, this serves the latest cached
// results instead of computing them live.
func (r *Registry) CheckReadiness(ctx context.Context) HealthStatus {
	r.mu.RLock()
	if r.started {
		status := r.snapshotStatus(probeReadiness)
		ready := r.ready
		r.mu.RUnlock()
		return applyReadyGate(status, ready)
	}
	checks := maps.Clone(r.checks)
	configs := maps.Clone(r.configs)
	ready := r.ready
	r.mu.RUnlock()

	// Add overall timeout for all readiness checks
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	status := r.performChecks(ctx, checks, configs, probeReadiness)
	return applyReadyGate(status, ready)
}

// applyReadyGate forces status to unhealthy when the service hasn't been
// explicitly marked ready, regardless of individual check results.
func applyReadyGate(status HealthStatus, ready bool) HealthStatus {
	if !ready {
		status.Status = StatusUnhealthy
		status.Message = "Service not marked as ready"
	}
	return status
}

// CheckHealth performs both liveness and readiness checks and returns combined results.
//
// When the background runner is active, both sides are built from a single
// cached snapshot (one lock acquisition) rather than two serialized live
// rounds of every check.
func (r *Registry) CheckHealth(ctx context.Context) HealthStatus {
	r.mu.RLock()
	if r.started {
		liveness := r.snapshotStatus(probeLiveness)
		readiness := applyReadyGate(r.snapshotStatus(probeReadiness), r.ready)
		r.mu.RUnlock()
		return combineHealth(liveness, readiness)
	}
	r.mu.RUnlock()

	liveness := r.CheckLiveness(ctx)
	readiness := r.CheckReadiness(ctx)
	return combineHealth(liveness, readiness)
}

// combineHealth merges a liveness and a readiness HealthStatus into the
// combined status returned by CheckHealth.
func combineHealth(liveness, readiness HealthStatus) HealthStatus {
	status := HealthStatus{
		Status:    StatusHealthy,
		Message:   "All checks passed",
		Timestamp: time.Now().UTC(),
		Details: map[string]CheckResult{
			"liveness":  {Name: "liveness", Status: liveness.Status, Message: liveness.Message, Timestamp: liveness.Timestamp},
			"readiness": {Name: "readiness", Status: readiness.Status, Message: readiness.Message, Timestamp: readiness.Timestamp},
		},
	}

	// Add individual check details
	for name, result := range liveness.Details {
		status.Details[name+"_liveness"] = result
	}
	for name, result := range readiness.Details {
		status.Details[name+"_readiness"] = result
	}

	// Overall status is unhealthy if either liveness or readiness fails
	if liveness.Status != StatusHealthy || readiness.Status != StatusHealthy {
		status.Status = StatusUnhealthy
		status.Message = "One or more checks failed"
	}

	return status
}

// snapshotStatus builds a HealthStatus from the cached background results
// for the given probe kind. Callers must hold at least r.mu.RLock.
//
// A check that the background runner hasn't completed a round for yet is
// reported as pending (StatusUnknown), which counts as failing for a
// required check - this is what keeps a pod from being reported ready
// before its first real check has run.
func (r *Registry) snapshotStatus(kind *probeKind) HealthStatus {
	if len(r.checks) == 0 {
		return HealthStatus{
			Status:    StatusHealthy,
			Message:   "No health checks registered",
			Timestamp: time.Now().UTC(),
			Details:   make(map[string]CheckResult),
		}
	}

	results := make(map[string]CheckResult, len(r.checks))
	overallHealthy := true

	for name, config := range r.configs {
		entry, ok := r.cache[name]

		var result CheckResult
		switch {
		case !ok || !entry.ran:
			result = CheckResult{
				Name:      name,
				Status:    StatusUnknown,
				Message:   "pending first check",
				Timestamp: time.Now().UTC(),
				Required:  config.Required,
			}
		case kind == probeLiveness:
			result = entry.liveness
		default:
			result = entry.readiness
		}

		results[name] = result
		if result.Required && !result.IsHealthy() {
			overallHealthy = false
		}
	}

	status := StatusHealthy
	message := "All checks passed"
	if !overallHealthy {
		status = StatusUnhealthy
		message = "One or more required checks failed"
	}

	return HealthStatus{
		Status:    status,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Details:   results,
	}
}

// performChecks executes health checks live and returns aggregated results.
// This is the fallback path used when the background runner has not been
// started (unit tests, direct library use without Start).
func (r *Registry) performChecks(ctx context.Context, checks map[string]Check, configs map[string]CheckConfig, kind *probeKind) HealthStatus {
	if len(checks) == 0 {
		return HealthStatus{
			Status:    StatusHealthy,
			Message:   "No health checks registered",
			Timestamp: time.Now().UTC(),
			Details:   make(map[string]CheckResult),
		}
	}

	results := make(map[string]CheckResult)
	overallHealthy := true

	// Execute checks concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check Check, config CheckConfig) {
			defer wg.Done()

			start := time.Now()
			err := kind.fn(check)(ctx)
			duration := time.Since(start)
			result := NewCheckResult(name, config.Required, duration, err)

			mu.Lock()
			results[name] = result
			if result.Required && !result.IsHealthy() {
				overallHealthy = false
			}
			mu.Unlock()

			if err != nil {
				r.logger.Warn("health check failed", "name", name, "type", kind.label, "error", err, "duration", duration)
			} else {
				r.logger.Debug("health check passed", "name", name, "type", kind.label, "duration", duration)
			}
		}(name, check, configs[name])
	}

	wg.Wait()

	// Determine overall status
	status := StatusHealthy
	message := "All checks passed"

	if !overallHealthy {
		status = StatusUnhealthy
		message = "One or more required checks failed"
	}

	return HealthStatus{
		Status:    status,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Details:   results,
	}
}

// GetRegisteredChecks returns the names of all registered checks.
func (r *Registry) GetRegisteredChecks() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.checks))
	for name := range r.checks {
		names = append(names, name)
	}
	return names
}

// GetCheckConfig returns the configuration for a specific check.
func (r *Registry) GetCheckConfig(name string) (CheckConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.configs[name]
	return config, exists
}
