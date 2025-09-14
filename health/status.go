package health

import (
	"encoding/json"
	"time"
)

// HealthStatus represents the overall health status of a service,
// including aggregated results from individual health checks.
type HealthStatus struct {
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]CheckResult `json:"details,omitempty"`
	Service   string                 `json:"service,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Uptime    string                 `json:"uptime,omitempty"`
}

// IsHealthy returns true if the overall status is healthy.
func (h HealthStatus) IsHealthy() bool {
	return h.Status == StatusHealthy
}

// IsReady returns true if the status indicates the service is ready to serve requests.
// This is an alias for IsHealthy for readiness checks.
func (h HealthStatus) IsReady() bool {
	return h.IsHealthy()
}

// IsLive returns true if the status indicates the service is alive.
// For liveness checks, we consider the service live if it's not explicitly unhealthy.
func (h HealthStatus) IsLive() bool {
	return h.Status != StatusUnhealthy
}

// HTTPStatus returns the appropriate HTTP status code for this health status.
func (h HealthStatus) HTTPStatus() int {
	switch h.Status {
	case StatusHealthy:
		return 200 // OK
	case StatusUnhealthy:
		return 503 // Service Unavailable
	case StatusUnknown:
		return 503 // Service Unavailable
	default:
		return 500 // Internal Server Error
	}
}

// JSON returns the JSON representation of the health status.
func (h HealthStatus) JSON() ([]byte, error) {
	return json.Marshal(h)
}

// String returns a string representation of the health status.
func (h HealthStatus) String() string {
	data, err := h.JSON()
	if err != nil {
		return "{\"status\":\"error\",\"message\":\"failed to serialize health status\"}"
	}
	return string(data)
}

// FailedChecks returns a slice of check results that failed.
func (h HealthStatus) FailedChecks() []CheckResult {
	var failed []CheckResult
	for _, result := range h.Details {
		if !result.IsHealthy() {
			failed = append(failed, result)
		}
	}
	return failed
}

// RequiredFailedChecks returns a slice of required check results that failed.
func (h HealthStatus) RequiredFailedChecks() []CheckResult {
	var failed []CheckResult
	for _, result := range h.Details {
		if result.Required && !result.IsHealthy() {
			failed = append(failed, result)
		}
	}
	return failed
}

// HealthySummary provides a summary of healthy vs unhealthy checks.
type HealthySummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Required  int `json:"required"`
}

// GetHealthySummary returns a summary of the health check results.
func (h HealthStatus) GetHealthySummary() HealthySummary {
	summary := HealthySummary{
		Total: len(h.Details),
	}

	for _, result := range h.Details {
		if result.IsHealthy() {
			summary.Healthy++
		} else {
			summary.Unhealthy++
		}
		if result.Required {
			summary.Required++
		}
	}

	return summary
}

// WithService sets the service name on the health status.
func (h HealthStatus) WithService(service string) HealthStatus {
	h.Service = service
	return h
}

// WithVersion sets the version on the health status.
func (h HealthStatus) WithVersion(version string) HealthStatus {
	h.Version = version
	return h
}

// WithUptime sets the uptime on the health status.
func (h HealthStatus) WithUptime(uptime time.Duration) HealthStatus {
	h.Uptime = uptime.String()
	return h
}

// NewHealthyStatus creates a new healthy status with the given message.
func NewHealthyStatus(message string) HealthStatus {
	if message == "" {
		message = "Service is healthy"
	}
	return HealthStatus{
		Status:    StatusHealthy,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Details:   make(map[string]CheckResult),
	}
}

// NewUnhealthyStatus creates a new unhealthy status with the given message.
func NewUnhealthyStatus(message string) HealthStatus {
	if message == "" {
		message = "Service is unhealthy"
	}
	return HealthStatus{
		Status:    StatusUnhealthy,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Details:   make(map[string]CheckResult),
	}
}

// NewUnknownStatus creates a new unknown status with the given message.
func NewUnknownStatus(message string) HealthStatus {
	if message == "" {
		message = "Service health is unknown"
	}
	return HealthStatus{
		Status:    StatusUnknown,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Details:   make(map[string]CheckResult),
	}
}

// StatusResponse is a simplified response for basic health checks.
type StatusResponse struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

// NewStatusResponse creates a new status response from a health status.
func NewStatusResponse(status HealthStatus) StatusResponse {
	return StatusResponse{
		Status:  status.Status,
		Message: status.Message,
	}
}

// JSON returns the JSON representation of the status response.
func (s StatusResponse) JSON() ([]byte, error) {
	return json.Marshal(s)
}

// HTTPStatus returns the appropriate HTTP status code.
func (s StatusResponse) HTTPStatus() int {
	switch s.Status {
	case StatusHealthy:
		return 200
	case StatusUnhealthy:
		return 503
	default:
		return 500
	}
}