package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- BasicCheck tests ---

func TestBasicCheck_Name(t *testing.T) {
	cfg := DefaultCheckConfig("my-db")
	check := NewBasicCheck(cfg, nil, nil)
	if check.Name() != "my-db" {
		t.Errorf("expected name 'my-db', got %q", check.Name())
	}
}

func TestBasicCheck_Config(t *testing.T) {
	cfg := DefaultCheckConfig("my-db")
	cfg.Required = false
	check := NewBasicCheck(cfg, nil, nil)
	if check.Config().Name != "my-db" {
		t.Errorf("expected config name 'my-db'")
	}
	if check.Config().Required {
		t.Error("expected Required=false")
	}
}

func TestBasicCheck_Liveness_NilFunc(t *testing.T) {
	cfg := DefaultCheckConfig("check")
	check := NewBasicCheck(cfg, nil, nil)
	// nil livenessFunc should return nil (always healthy)
	if err := check.Liveness(context.Background()); err != nil {
		t.Errorf("expected nil error for nil liveness func, got: %v", err)
	}
}

func TestBasicCheck_Liveness_Success(t *testing.T) {
	cfg := DefaultCheckConfig("check")
	check := NewBasicCheck(cfg, func(ctx context.Context) error {
		return nil
	}, nil)

	if err := check.Liveness(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestBasicCheck_Liveness_Failure(t *testing.T) {
	cfg := DefaultCheckConfig("check")
	check := NewBasicCheck(cfg, func(ctx context.Context) error {
		return errors.New("connection refused")
	}, nil)

	if err := check.Liveness(context.Background()); err == nil {
		t.Error("expected error from liveness check")
	}
}

func TestBasicCheck_Readiness_NilFunc_FallsBackToLiveness(t *testing.T) {
	cfg := DefaultCheckConfig("check")
	livenessCalled := false
	check := NewBasicCheck(cfg, func(ctx context.Context) error {
		livenessCalled = true
		return nil
	}, nil) // nil readiness func

	if err := check.Readiness(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !livenessCalled {
		t.Error("expected liveness func to be called as fallback")
	}
}

func TestBasicCheck_Readiness_CustomFunc(t *testing.T) {
	cfg := DefaultCheckConfig("check")
	readinessCalled := false
	check := NewBasicCheck(cfg,
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error {
			readinessCalled = true
			return nil
		},
	)

	if err := check.Readiness(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !readinessCalled {
		t.Error("expected readiness func to be called")
	}
}

func TestBasicCheck_Readiness_Failure(t *testing.T) {
	cfg := DefaultCheckConfig("check")
	check := NewBasicCheck(cfg,
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return errors.New("not ready") },
	)

	if err := check.Readiness(context.Background()); err == nil {
		t.Error("expected error from readiness check")
	}
}

func TestBasicCheck_ZeroTimeout_UsesDefault(t *testing.T) {
	cfg := DefaultCheckConfig("check")
	cfg.Timeout = 0 // force the default path
	check := NewBasicCheck(cfg, func(ctx context.Context) error {
		return nil
	}, nil)

	if err := check.Liveness(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// --- AlwaysUnhealthyCheck tests ---

func TestAlwaysUnhealthyCheck_Name(t *testing.T) {
	check := NewAlwaysUnhealthyCheck("db", errors.New("timeout"))
	if check.Name() != "db" {
		t.Errorf("expected name 'db', got %q", check.Name())
	}
}

func TestAlwaysUnhealthyCheck_Liveness(t *testing.T) {
	check := NewAlwaysUnhealthyCheck("db", errors.New("always fails"))
	if err := check.Liveness(context.Background()); err == nil {
		t.Error("expected error from always-unhealthy check")
	}
}

func TestAlwaysUnhealthyCheck_Readiness(t *testing.T) {
	check := NewAlwaysUnhealthyCheck("db", errors.New("always fails"))
	if err := check.Readiness(context.Background()); err == nil {
		t.Error("expected error from always-unhealthy readiness check")
	}
}

func TestAlwaysUnhealthyCheck_NilError_UsesDeadlineExceeded(t *testing.T) {
	check := NewAlwaysUnhealthyCheck("db", nil)
	err := check.Liveness(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// --- NewCheckResult tests ---

func TestNewCheckResult_Healthy(t *testing.T) {
	result := NewCheckResult("db", true, 5*time.Millisecond, nil)
	if result.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %q", result.Status)
	}
	if result.Message != "OK" {
		t.Errorf("expected message 'OK', got %q", result.Message)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
	if !result.Required {
		t.Error("expected Required=true")
	}
	if !result.IsHealthy() {
		t.Error("expected IsHealthy()=true")
	}
}

func TestNewCheckResult_Unhealthy(t *testing.T) {
	result := NewCheckResult("db", false, 50*time.Millisecond, errors.New("connection failed"))
	if result.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status, got %q", result.Status)
	}
	if result.Error != "connection failed" {
		t.Errorf("expected error 'connection failed', got %q", result.Error)
	}
	if result.IsHealthy() {
		t.Error("expected IsHealthy()=false")
	}
}

// --- DefaultCheckConfig tests ---

func TestDefaultCheckConfig(t *testing.T) {
	cfg := DefaultCheckConfig("database")
	if cfg.Name != "database" {
		t.Errorf("expected name 'database', got %q", cfg.Name)
	}
	if !cfg.Required {
		t.Error("expected Required=true by default")
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", cfg.Timeout)
	}
}
