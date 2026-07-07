package forgeerrors

import (
	"errors"
	"fmt"
	"testing"
)

// TestDomainError_Error tests basic error message formatting
func TestDomainError_Error(t *testing.T) {
	err := DomainError{
		Code:    "TEST_ERROR",
		Message: "test message",
	}

	expected := "TEST_ERROR: test message"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

// TestDomainError_Error_WithCause tests error message with cause
func TestDomainError_Error_WithCause(t *testing.T) {
	cause := errors.New("underlying error")
	err := DomainError{
		Code:    "TEST_ERROR",
		Message: "test message",
		Cause:   cause,
	}

	errorStr := err.Error()
	if !contains(errorStr, "TEST_ERROR") {
		t.Error("Expected error to contain code")
	}
	if !contains(errorStr, "test message") {
		t.Error("Expected error to contain message")
	}
	if !contains(errorStr, "underlying error") {
		t.Error("Expected error to contain cause")
	}
}

// TestDomainError_WithMessage tests creating error with formatted message
func TestDomainError_WithMessage(t *testing.T) {
	base := DomainError{
		Code:    "TEST_ERROR",
		Message: "base message",
	}

	err := base.WithMessage("formatted %s with %d", "message", 42)

	if err.Code != "TEST_ERROR" {
		t.Errorf("Expected code to be preserved, got %s", err.Code)
	}

	expected := "formatted message with 42"
	if err.Message != expected {
		t.Errorf("Expected message '%s', got '%s'", expected, err.Message)
	}
}

// TestDomainError_WithCause tests adding cause to error
func TestDomainError_WithCause(t *testing.T) {
	base := DomainError{
		Code:    "TEST_ERROR",
		Message: "test message",
	}

	cause := errors.New("root cause")
	err := base.WithCause(cause)

	if err.Code != "TEST_ERROR" {
		t.Error("Expected code to be preserved")
	}

	if err.Message != "test message" {
		t.Error("Expected message to be preserved")
	}

	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

// TestDomainError_Unwrap tests error unwrapping
func TestDomainError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := DomainError{
		Code:    "TEST_ERROR",
		Message: "test message",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Error("Expected Unwrap to return the cause")
	}
}

// TestDomainError_ErrorsIs tests errors.Is compatibility
func TestDomainError_ErrorsIs(t *testing.T) {
	rootErr := errors.New("root error")
	domainErr := ErrRepositoryUnavailable.WithCause(rootErr)

	if !errors.Is(domainErr, rootErr) {
		t.Error("Expected errors.Is to work with wrapped errors")
	}
}

// TestIsTransientError tests transient error classification
func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{"nil error", nil, false},
		{"service unavailable", ErrServiceUnavailable, true},
		{"service timeout", ErrServiceTimeout, true},
		{"rate limit exceeded", ErrRateLimitExceeded, true},
		{"repository unavailable", ErrRepositoryUnavailable, true},
		{"authentication failed", ErrAuthenticationFailed, false},
		{"invalid config", ErrInvalidConfiguration, false},
		{"standard error", errors.New("standard error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransientError(tt.err)
			if result != tt.transient {
				t.Errorf("Expected IsTransientError(%v) = %v, got %v", tt.err, tt.transient, result)
			}
		})
	}
}

// TestIsAuthenticationError tests authentication error classification
func TestIsAuthenticationError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		isAuth bool
	}{
		{"nil error", nil, false},
		{"authentication failed", ErrAuthenticationFailed, true},
		{"invalid credential", ErrInvalidCredential, true},
		{"insufficient permissions", ErrInsufficientPermissions, true},
		{"service unavailable", ErrServiceUnavailable, false},
		{"invalid config", ErrInvalidConfiguration, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAuthenticationError(tt.err)
			if result != tt.isAuth {
				t.Errorf("Expected IsAuthenticationError(%v) = %v, got %v", tt.err, tt.isAuth, result)
			}
		})
	}
}

// TestIsValidationError tests validation error classification
func TestIsValidationError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		isValidation bool
	}{
		{"nil error", nil, false},
		{"invalid configuration", ErrInvalidConfiguration, true},
		{"invalid event payload", ErrInvalidEventPayload, true},
		{"authentication failed", ErrAuthenticationFailed, false},
		{"service unavailable", ErrServiceUnavailable, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidationError(tt.err)
			if result != tt.isValidation {
				t.Errorf("Expected IsValidationError(%v) = %v, got %v", tt.err, tt.isValidation, result)
			}
		})
	}
}

// TestIsConfigurationError tests configuration error classification
func TestIsConfigurationError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		isConfigErr bool
	}{
		{"nil error", nil, false},
		{"missing configuration", ErrMissingConfiguration, true},
		{"invalid configuration", ErrInvalidConfiguration, true},
		{"authentication failed", ErrAuthenticationFailed, false},
		{"repository unavailable", ErrRepositoryUnavailable, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsConfigurationError(tt.err)
			if result != tt.isConfigErr {
				t.Errorf("Expected IsConfigurationError(%v) = %v, got %v", tt.err, tt.isConfigErr, result)
			}
		})
	}
}

// TestDomainError_Is tests the Is() method for code-based equality
func TestDomainError_Is(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "same code different message",
			err:    ErrRepositoryUnavailable.WithMessage("database connection failed"),
			target: ErrRepositoryUnavailable,
			want:   true,
		},
		{
			name:   "same code with cause",
			err:    ErrRepositoryUnavailable.WithCause(errors.New("timeout")),
			target: ErrRepositoryUnavailable,
			want:   true,
		},
		{
			name:   "different codes",
			err:    ErrRepositoryUnavailable,
			target: ErrAuthenticationFailed,
			want:   false,
		},
		{
			name:   "target is non-domain error",
			err:    ErrRepositoryUnavailable,
			target: errors.New("plain error"),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err, tt.target)
			if got != tt.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}

// TestDomainError_Is_ThroughWrappingChain tests errors.Is works through stdlib fmt.Errorf wrapping
func TestDomainError_Is_ThroughWrappingChain(t *testing.T) {
	base := ErrRepositoryUnavailable.WithMessage("db unavailable")
	wrapped := fmt.Errorf("operation failed: %w", base)

	if !errors.Is(wrapped, ErrRepositoryUnavailable) {
		t.Error("expected errors.Is to match through fmt.Errorf wrapping chain")
	}

	if errors.Is(wrapped, ErrAuthenticationFailed) {
		t.Error("expected errors.Is to NOT match different code through wrapping chain")
	}
}

// TestIsRepositoryError tests repository error classification
func TestIsRepositoryError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		isRepoErr bool
	}{
		{"nil error", nil, false},
		{"repository unavailable", ErrRepositoryUnavailable, true},
		{"transaction failed", ErrTransactionFailed, true},
		{"constraint violation", ErrConstraintViolation, true},
		{"service unavailable", ErrServiceUnavailable, false},
		{"authentication failed", ErrAuthenticationFailed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRepositoryError(tt.err)
			if result != tt.isRepoErr {
				t.Errorf("Expected IsRepositoryError(%v) = %v, got %v", tt.err, tt.isRepoErr, result)
			}
		})
	}
}

// TestErrorClassification_WithWrappedErrors tests classification works with wrapped errors
func TestErrorClassification_WithWrappedErrors(t *testing.T) {
	rootErr := errors.New("database connection failed")
	wrappedErr := ErrRepositoryUnavailable.WithCause(rootErr)

	if !IsRepositoryError(wrappedErr) {
		t.Error("Expected IsRepositoryError to work with wrapped errors")
	}

	if !IsTransientError(wrappedErr) {
		t.Error("Expected IsTransientError to work with wrapped errors")
	}
}

// TestPredefinedErrors_Structure tests all predefined errors have proper structure
func TestPredefinedErrors_Structure(t *testing.T) {
	predefinedErrors := []DomainError{
		ErrInvalidConfiguration,
		ErrMissingConfiguration,
		ErrRepositoryUnavailable,
		ErrTransactionFailed,
		ErrConstraintViolation,
		ErrEventPublishingFailed,
		ErrInvalidEventPayload,
		ErrRateLimitExceeded,
		ErrQuotaExceeded,
		ErrAuthenticationFailed,
		ErrInvalidCredential,
		ErrInsufficientPermissions,
		ErrServiceUnavailable,
		ErrServiceTimeout,
		ErrServiceOperationFailed,
	}

	for _, err := range predefinedErrors {
		if err.Code == "" {
			t.Errorf("Error %v has empty code", err)
		}
		if err.Message == "" {
			t.Errorf("Error %v has empty message", err)
		}
		// Predefined errors should not have a cause initially
		if err.Cause != nil {
			t.Errorf("Error %v should not have initial cause", err)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
