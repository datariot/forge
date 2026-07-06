package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Registry manages a collection of health checks and provides methods
// to check the overall health status of a service.
type Registry struct {
	mu      sync.RWMutex
	checks  map[string]Check
	configs map[string]CheckConfig
	ready   bool
	logger  Logger
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
	}
}

// Register registers a health check with the registry.
func (r *Registry) Register(check Check, config CheckConfig) error {
	if check == nil {
		return fmt.Errorf("check cannot be nil")
	}

	name := check.Name()
	if name == "" {
		return fmt.Errorf("check name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.checks[name]; exists {
		return fmt.Errorf("check with name %q already registered", name)
	}

	r.checks[name] = check
	r.configs[name] = config

	r.logger.Info("registered health check", "name", name, "required", config.Required)
	return nil
}

// MustRegister registers a health check and panics if registration fails.
func (r *Registry) MustRegister(check Check, config CheckConfig) {
	if err := r.Register(check, config); err != nil {
		panic(fmt.Sprintf("failed to register health check: %v", err))
	}
}

// Unregister removes a health check from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.checks, name)
	delete(r.configs, name)

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

// CheckLiveness performs liveness checks on all registered health checks.
func (r *Registry) CheckLiveness(ctx context.Context) HealthStatus {
	// Add overall timeout for all liveness checks
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	r.mu.RLock()
	checks := make(map[string]Check, len(r.checks))
	configs := make(map[string]CheckConfig, len(r.configs))
	for name, check := range r.checks {
		checks[name] = check
		configs[name] = r.configs[name]
	}
	r.mu.RUnlock()

	return r.performChecks(ctx, checks, configs, "liveness")
}

// CheckReadiness performs readiness checks on all registered health checks.
func (r *Registry) CheckReadiness(ctx context.Context) HealthStatus {
	// Add overall timeout for all readiness checks
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	r.mu.RLock()
	checks := make(map[string]Check, len(r.checks))
	configs := make(map[string]CheckConfig, len(r.configs))
	ready := r.ready
	for name, check := range r.checks {
		checks[name] = check
		configs[name] = r.configs[name]
	}
	r.mu.RUnlock()

	status := r.performChecks(ctx, checks, configs, "readiness")

	// If the service hasn't been marked as ready, it's not ready regardless of checks
	if !ready {
		status.Status = StatusUnhealthy
		status.Message = "Service not marked as ready"
	}

	return status
}

// CheckHealth performs both liveness and readiness checks and returns combined results.
func (r *Registry) CheckHealth(ctx context.Context) HealthStatus {
	liveness := r.CheckLiveness(ctx)
	readiness := r.CheckReadiness(ctx)

	// Combine results - service is healthy if both liveness and readiness pass
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

// performChecks executes health checks and returns aggregated results.
func (r *Registry) performChecks(ctx context.Context, checks map[string]Check, configs map[string]CheckConfig, checkType string) HealthStatus {
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
			var err error

			switch checkType {
			case "liveness":
				err = check.Liveness(ctx)
			case "readiness":
				err = check.Readiness(ctx)
			default:
				err = fmt.Errorf("unknown check type: %s", checkType)
			}

			duration := time.Since(start)
			result := NewCheckResult(name, config.Required, duration, err)

			mu.Lock()
			results[name] = result
			if result.Required && !result.IsHealthy() {
				overallHealthy = false
			}
			mu.Unlock()

			if err != nil {
				r.logger.Warn("health check failed", "name", name, "type", checkType, "error", err, "duration", duration)
			} else {
				r.logger.Debug("health check passed", "name", name, "type", checkType, "duration", duration)
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
