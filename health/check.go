package health

import (
	"context"
	"time"
)

// Check represents a health check that can be performed on a service dependency.
// Each check supports both liveness (is the dependency alive) and readiness
// (is the dependency ready to serve requests) semantics.
type Check interface {
	// Name returns a unique name for this health check.
	Name() string

	// Liveness performs a liveness check. This should be a fast, lightweight
	// check that indicates whether the dependency is alive and responding.
	// A failed liveness check typically indicates a need for service restart.
	Liveness(ctx context.Context) error

	// Readiness performs a readiness check. This can be more comprehensive
	// than liveness and indicates whether the dependency is ready to serve
	// requests. A failed readiness check should result in the service being
	// marked as not ready (but still alive).
	Readiness(ctx context.Context) error
}

// CheckConfig contains common configuration for health checks.
type CheckConfig struct {
	// Name is the unique name of the health check
	Name string

	// Required indicates whether this check is required for readiness.
	// If true, the service will not be marked as ready until this check passes.
	Required bool

	// Timeout is the maximum time to wait for the health check to complete.
	Timeout time.Duration

	// Interval is how often to perform the check (for periodic checks).
	Interval time.Duration

	// InitialDelay is how long to wait before performing the first check.
	InitialDelay time.Duration
}

// DefaultCheckConfig returns a CheckConfig with sensible defaults.
func DefaultCheckConfig(name string) CheckConfig {
	return CheckConfig{
		Name:         name,
		Required:     true,
		Timeout:      5 * time.Second,
		Interval:     30 * time.Second,
		InitialDelay: 0,
	}
}

// Status represents the status of a health check.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusUnknown   Status = "unknown"
)

// CheckResult represents the result of executing a health check.
type CheckResult struct {
	Name      string    `json:"name"`
	Status    Status    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	Duration  string    `json:"duration"`
	Timestamp time.Time `json:"timestamp"`
	Required  bool      `json:"required"`
}

// NewCheckResult creates a new CheckResult.
func NewCheckResult(name string, required bool, duration time.Duration, err error) CheckResult {
	result := CheckResult{
		Name:      name,
		Duration:  duration.String(),
		Timestamp: time.Now().UTC(),
		Required:  required,
	}

	if err != nil {
		result.Status = StatusUnhealthy
		result.Error = err.Error()
	} else {
		result.Status = StatusHealthy
		result.Message = "OK"
	}

	return result
}

// IsHealthy returns true if the check result indicates a healthy status.
func (r CheckResult) IsHealthy() bool {
	return r.Status == StatusHealthy
}

// BasicCheck provides a simple implementation of Check interface.
type BasicCheck struct {
	config        CheckConfig
	livenessFunc  func(ctx context.Context) error
	readinessFunc func(ctx context.Context) error
}

// NewBasicCheck creates a new BasicCheck.
func NewBasicCheck(config CheckConfig, liveness, readiness func(ctx context.Context) error) *BasicCheck {
	return &BasicCheck{
		config:        config,
		livenessFunc:  liveness,
		readinessFunc: readiness,
	}
}

// Name returns the name of the health check.
func (c *BasicCheck) Name() string {
	return c.config.Name
}

// Liveness performs the liveness check.
func (c *BasicCheck) Liveness(ctx context.Context) error {
	if c.livenessFunc == nil {
		return nil
	}

	timeout := c.config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.livenessFunc(ctx)
}

// Readiness performs the readiness check.
func (c *BasicCheck) Readiness(ctx context.Context) error {
	if c.readinessFunc == nil {
		// Default to liveness if no readiness function provided
		return c.Liveness(ctx)
	}

	timeout := c.config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.readinessFunc(ctx)
}

// Config returns the check configuration.
func (c *BasicCheck) Config() CheckConfig {
	return c.config
}

// AlwaysHealthyCheck is a check that always reports healthy status.
// Useful for testing or as a placeholder.
type AlwaysHealthyCheck struct {
	name string
}

// NewAlwaysHealthyCheck creates a check that always reports healthy.
func NewAlwaysHealthyCheck(name string) *AlwaysHealthyCheck {
	return &AlwaysHealthyCheck{name: name}
}

// Name returns the name of the check.
func (c *AlwaysHealthyCheck) Name() string {
	return c.name
}

// Liveness always returns nil (healthy).
func (c *AlwaysHealthyCheck) Liveness(ctx context.Context) error {
	return nil
}

// Readiness always returns nil (healthy).
func (c *AlwaysHealthyCheck) Readiness(ctx context.Context) error {
	return nil
}

// AlwaysUnhealthyCheck is a check that always reports unhealthy status.
// Useful for testing failure scenarios.
type AlwaysUnhealthyCheck struct {
	name string
	err  error
}

// NewAlwaysUnhealthyCheck creates a check that always reports unhealthy.
func NewAlwaysUnhealthyCheck(name string, err error) *AlwaysUnhealthyCheck {
	if err == nil {
		err = context.DeadlineExceeded
	}
	return &AlwaysUnhealthyCheck{name: name, err: err}
}

// Name returns the name of the check.
func (c *AlwaysUnhealthyCheck) Name() string {
	return c.name
}

// Liveness always returns an error (unhealthy).
func (c *AlwaysUnhealthyCheck) Liveness(ctx context.Context) error {
	return c.err
}

// Readiness always returns an error (unhealthy).
func (c *AlwaysUnhealthyCheck) Readiness(ctx context.Context) error {
	return c.err
}