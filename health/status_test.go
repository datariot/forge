package health

import (
	"strings"
	"testing"
	"time"
)

func TestHealthStatus_IsHealthy(t *testing.T) {
	healthy := NewHealthyStatus("all good")
	if !healthy.IsHealthy() {
		t.Error("expected healthy status to be healthy")
	}

	unhealthy := NewUnhealthyStatus("broken")
	if unhealthy.IsHealthy() {
		t.Error("expected unhealthy status to not be healthy")
	}
}

func TestHealthStatus_IsReady(t *testing.T) {
	healthy := NewHealthyStatus("")
	if !healthy.IsReady() {
		t.Error("expected healthy status to be ready")
	}
}

func TestHealthStatus_IsLive(t *testing.T) {
	healthy := NewHealthyStatus("")
	if !healthy.IsLive() {
		t.Error("expected healthy status to be live")
	}

	unknown := NewUnknownStatus("")
	if !unknown.IsLive() {
		t.Error("expected unknown status to be live (not explicitly unhealthy)")
	}

	unhealthy := NewUnhealthyStatus("")
	if unhealthy.IsLive() {
		t.Error("expected unhealthy status to not be live")
	}
}

func TestHealthStatus_HTTPStatus(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected int
	}{
		{NewHealthyStatus(""), 200},
		{NewUnhealthyStatus(""), 503},
		{NewUnknownStatus(""), 503},
	}
	for _, tt := range tests {
		code := tt.status.HTTPStatus()
		if code != tt.expected {
			t.Errorf("status=%q: expected HTTP %d, got %d", tt.status.Status, tt.expected, code)
		}
	}
}

func TestHealthStatus_JSON(t *testing.T) {
	status := NewHealthyStatus("all good")
	data, err := status.JSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "healthy") {
		t.Errorf("expected JSON to contain 'healthy', got: %s", data)
	}
}

func TestHealthStatus_String(t *testing.T) {
	status := NewHealthyStatus("ok")
	s := status.String()
	if !strings.Contains(s, "healthy") {
		t.Errorf("expected String() to contain 'healthy', got: %s", s)
	}
}

func TestHealthStatus_FailedChecks(t *testing.T) {
	status := NewHealthyStatus("")
	status.Details["db"] = NewCheckResult("db", true, time.Millisecond, nil)
	status.Details["redis"] = NewCheckResult("redis", false, time.Millisecond, errTest("connection refused"))

	failed := status.FailedChecks()
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed check, got %d", len(failed))
	}
	if failed[0].Name != "redis" {
		t.Errorf("expected failed check 'redis', got %q", failed[0].Name)
	}
}

func TestHealthStatus_RequiredFailedChecks(t *testing.T) {
	status := NewHealthyStatus("")
	// required, healthy
	status.Details["db"] = NewCheckResult("db", true, time.Millisecond, nil)
	// required, unhealthy
	status.Details["redis"] = NewCheckResult("redis", true, time.Millisecond, errTest("failed"))
	// not required, unhealthy
	status.Details["cache"] = NewCheckResult("cache", false, time.Millisecond, errTest("failed"))

	required := status.RequiredFailedChecks()
	if len(required) != 1 {
		t.Fatalf("expected 1 required failed check, got %d", len(required))
	}
	if required[0].Name != "redis" {
		t.Errorf("expected 'redis', got %q", required[0].Name)
	}
}

func TestHealthStatus_GetHealthySummary(t *testing.T) {
	status := NewHealthyStatus("")
	status.Details["db"] = NewCheckResult("db", true, time.Millisecond, nil)
	status.Details["redis"] = NewCheckResult("redis", true, time.Millisecond, errTest("failed"))
	status.Details["cache"] = NewCheckResult("cache", false, time.Millisecond, nil)

	summary := status.GetHealthySummary()
	if summary.Total != 3 {
		t.Errorf("expected Total=3, got %d", summary.Total)
	}
	if summary.Healthy != 2 {
		t.Errorf("expected Healthy=2, got %d", summary.Healthy)
	}
	if summary.Unhealthy != 1 {
		t.Errorf("expected Unhealthy=1, got %d", summary.Unhealthy)
	}
	if summary.Required != 2 {
		t.Errorf("expected Required=2, got %d", summary.Required)
	}
}

func TestHealthStatus_WithService(t *testing.T) {
	status := NewHealthyStatus("").WithService("my-service")
	if status.Service != "my-service" {
		t.Errorf("expected Service='my-service', got %q", status.Service)
	}
}

func TestHealthStatus_WithVersion(t *testing.T) {
	status := NewHealthyStatus("").WithVersion("1.2.3")
	if status.Version != "1.2.3" {
		t.Errorf("expected Version='1.2.3', got %q", status.Version)
	}
}

func TestHealthStatus_WithUptime(t *testing.T) {
	status := NewHealthyStatus("").WithUptime(5 * time.Minute)
	if status.Uptime == "" {
		t.Error("expected non-empty Uptime")
	}
}

func TestNewHealthyStatus_EmptyMessage(t *testing.T) {
	status := NewHealthyStatus("")
	if status.Message == "" {
		t.Error("expected default message for empty input")
	}
}

func TestNewUnhealthyStatus_EmptyMessage(t *testing.T) {
	status := NewUnhealthyStatus("")
	if status.Message == "" {
		t.Error("expected default message for empty input")
	}
}

func TestNewUnknownStatus_EmptyMessage(t *testing.T) {
	status := NewUnknownStatus("")
	if status.Message == "" {
		t.Error("expected default message for empty input")
	}
}

func TestStatusResponse_JSON(t *testing.T) {
	status := NewHealthyStatus("ok")
	resp := NewStatusResponse(status)

	data, err := resp.JSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "healthy") {
		t.Errorf("expected JSON to contain 'healthy', got: %s", data)
	}
}

func TestStatusResponse_HTTPStatus(t *testing.T) {
	tests := []struct {
		statusResp StatusResponse
		expected   int
	}{
		{StatusResponse{Status: StatusHealthy}, 200},
		{StatusResponse{Status: StatusUnhealthy}, 503},
		{StatusResponse{Status: "something-else"}, 500},
	}
	for _, tt := range tests {
		code := tt.statusResp.HTTPStatus()
		if code != tt.expected {
			t.Errorf("status=%q: expected %d, got %d", tt.statusResp.Status, tt.expected, code)
		}
	}
}

// errTest is a simple error type for tests.
type errTest string

func (e errTest) Error() string { return string(e) }
