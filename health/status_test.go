package health

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReport_IsHealthy(t *testing.T) {
	healthy := NewHealthyReport("all good")
	if !healthy.IsHealthy() {
		t.Error("expected healthy status to be healthy")
	}

	unhealthy := NewUnhealthyReport("broken")
	if unhealthy.IsHealthy() {
		t.Error("expected unhealthy status to not be healthy")
	}
}

func TestReport_IsReady(t *testing.T) {
	healthy := NewHealthyReport("")
	if !healthy.IsReady() {
		t.Error("expected healthy status to be ready")
	}
}

func TestReport_IsLive(t *testing.T) {
	healthy := NewHealthyReport("")
	if !healthy.IsLive() {
		t.Error("expected healthy status to be live")
	}

	unknown := NewUnknownReport("")
	if !unknown.IsLive() {
		t.Error("expected unknown status to be live (not explicitly unhealthy)")
	}

	unhealthy := NewUnhealthyReport("")
	if unhealthy.IsLive() {
		t.Error("expected unhealthy status to not be live")
	}
}

func TestReport_HTTPStatus(t *testing.T) {
	tests := []struct {
		status   Report
		expected int
	}{
		{NewHealthyReport(""), 200},
		{NewUnhealthyReport(""), 503},
		{NewUnknownReport(""), 503},
	}
	for _, tt := range tests {
		code := tt.status.HTTPStatus()
		if code != tt.expected {
			t.Errorf("status=%q: expected HTTP %d, got %d", tt.status.Status, tt.expected, code)
		}
	}
}

func TestReport_JSON(t *testing.T) {
	status := NewHealthyReport("all good")
	data, err := status.JSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "healthy") {
		t.Errorf("expected JSON to contain 'healthy', got: %s", data)
	}
}

func TestReport_String(t *testing.T) {
	status := NewHealthyReport("ok")
	s := status.String()
	if !strings.Contains(s, "healthy") {
		t.Errorf("expected String() to contain 'healthy', got: %s", s)
	}
}

func TestReport_FailedChecks(t *testing.T) {
	status := NewHealthyReport("")
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

func TestReport_RequiredFailedChecks(t *testing.T) {
	status := NewHealthyReport("")
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

func TestReport_HealthySummary(t *testing.T) {
	status := NewHealthyReport("")
	status.Details["db"] = NewCheckResult("db", true, time.Millisecond, nil)
	status.Details["redis"] = NewCheckResult("redis", true, time.Millisecond, errTest("failed"))
	status.Details["cache"] = NewCheckResult("cache", false, time.Millisecond, nil)

	summary := status.HealthySummary()
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

func TestReport_WithService(t *testing.T) {
	status := NewHealthyReport("").WithService("my-service")
	if status.Service != "my-service" {
		t.Errorf("expected Service='my-service', got %q", status.Service)
	}
}

func TestReport_WithVersion(t *testing.T) {
	status := NewHealthyReport("").WithVersion("1.2.3")
	if status.Version != "1.2.3" {
		t.Errorf("expected Version='1.2.3', got %q", status.Version)
	}
}

func TestReport_WithUptime(t *testing.T) {
	status := NewHealthyReport("").WithUptime(5 * time.Minute)
	if status.Uptime == "" {
		t.Error("expected non-empty Uptime")
	}
}

func TestNewHealthyReport_EmptyMessage(t *testing.T) {
	status := NewHealthyReport("")
	if status.Message == "" {
		t.Error("expected default message for empty input")
	}
}

func TestNewUnhealthyReport_EmptyMessage(t *testing.T) {
	status := NewUnhealthyReport("")
	if status.Message == "" {
		t.Error("expected default message for empty input")
	}
}

func TestNewUnknownReport_EmptyMessage(t *testing.T) {
	status := NewUnknownReport("")
	if status.Message == "" {
		t.Error("expected default message for empty input")
	}
}

func TestStatusResponse_JSON(t *testing.T) {
	status := NewHealthyReport("ok")
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

func TestCheckResult_IsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected bool
	}{
		{"healthy check result", StatusHealthy, true},
		{"unhealthy check result", StatusUnhealthy, false},
		{"unknown check result", StatusUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckResult{
				Name:   "test-check",
				Status: tt.status,
			}

			if result.IsHealthy() != tt.expected {
				t.Errorf("Expected IsHealthy() = %v, got %v", tt.expected, result.IsHealthy())
			}
		})
	}
}

func TestStatusResponse_Creation(t *testing.T) {
	hs := Report{
		Status:  StatusHealthy,
		Message: "test message",
	}

	resp := NewStatusResponse(hs)

	if resp.Status != StatusHealthy {
		t.Errorf("Expected status healthy, got %s", resp.Status)
	}

	if resp.Message != "test message" {
		t.Errorf("Expected message 'test message', got %s", resp.Message)
	}
}

// --- Report.Redacted tests ---

func TestReport_Redacted_StripsCheckErrors(t *testing.T) {
	status := Report{
		Status:  StatusUnhealthy,
		Message: "One or more checks failed",
		Details: map[string]CheckResult{
			"postgres": NewCheckResult("postgres", true, time.Millisecond, errors.New("dial tcp 10.0.1.5:5432: connect: connection refused")),
			"cache":    NewCheckResult("cache", false, time.Millisecond, nil),
		},
	}

	redacted := status.Redacted()

	if redacted.Status != status.Status {
		t.Errorf("expected overall Status preserved, got %q", redacted.Status)
	}
	if redacted.Message != status.Message {
		t.Errorf("expected overall Message preserved, got %q", redacted.Message)
	}

	for name, result := range redacted.Details {
		if result.Error != "" {
			t.Errorf("expected redacted Details[%q].Error to be empty, got %q", name, result.Error)
		}
	}

	if status.Details["postgres"].Error == "" {
		t.Error("expected original status Details to remain unaffected")
	}
}

func TestReport_Redacted_NoDetails(t *testing.T) {
	status := NewHealthyReport("")

	redacted := status.Redacted()

	if len(redacted.Details) != 0 {
		t.Errorf("expected no details, got %d", len(redacted.Details))
	}
}
