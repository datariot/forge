package errors

import (
	"errors"
	"fmt"
)

// DomainError represents a domain-specific error with additional context
type DomainError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"cause,omitempty"`
}

// Error implements the error interface
func (e DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WithMessage returns a new DomainError with the specified message
func (e DomainError) WithMessage(format string, args ...interface{}) DomainError {
	return DomainError{
		Code:    e.Code,
		Message: fmt.Sprintf(format, args...),
		Cause:   e.Cause,
	}
}

// WithCause returns a new DomainError with the specified cause
func (e DomainError) WithCause(cause error) DomainError {
	return DomainError{
		Code:    e.Code,
		Message: e.Message,
		Cause:   cause,
	}
}

// Unwrap returns the underlying cause for error unwrapping
func (e DomainError) Unwrap() error {
	return e.Cause
}

// Common domain error definitions shared across services
var (
	// Configuration Errors (shared across services)
	ErrInvalidConfiguration = DomainError{
		Code:    "INVALID_CONFIGURATION",
		Message: "configuration is invalid",
	}
	ErrMissingConfiguration = DomainError{
		Code:    "MISSING_CONFIGURATION",
		Message: "required configuration is missing",
	}

	// Repository Errors (shared database operations)
	ErrRepositoryUnavailable = DomainError{
		Code:    "REPOSITORY_UNAVAILABLE",
		Message: "repository is unavailable",
	}
	ErrTransactionFailed = DomainError{
		Code:    "TRANSACTION_FAILED",
		Message: "database transaction failed",
	}
	ErrConstraintViolation = DomainError{
		Code:    "CONSTRAINT_VIOLATION",
		Message: "database constraint violation",
	}

	// Event Publishing Errors (shared Redis/event operations)
	ErrEventPublishingFailed = DomainError{
		Code:    "EVENT_PUBLISHING_FAILED",
		Message: "failed to publish event",
	}
	ErrInvalidEventPayload = DomainError{
		Code:    "INVALID_EVENT_PAYLOAD",
		Message: "event payload is invalid",
	}

	// Rate Limiting Errors
	ErrRateLimitExceeded = DomainError{
		Code:    "RATE_LIMIT_EXCEEDED",
		Message: "API rate limit exceeded",
	}
	ErrQuotaExceeded = DomainError{
		Code:    "QUOTA_EXCEEDED",
		Message: "API quota exceeded",
	}

	// Authentication Errors
	ErrAuthenticationFailed = DomainError{
		Code:    "AUTHENTICATION_FAILED",
		Message: "authentication failed",
	}
	ErrInvalidCredential = DomainError{
		Code:    "INVALID_CREDENTIAL",
		Message: "credential is invalid or expired",
	}
	ErrInsufficientPermissions = DomainError{
		Code:    "INSUFFICIENT_PERMISSIONS",
		Message: "insufficient permissions for requested operation",
	}

	// Service Communication Errors
	ErrServiceUnavailable = DomainError{
		Code:    "SERVICE_UNAVAILABLE",
		Message: "external service is unavailable",
	}
	ErrServiceTimeout = DomainError{
		Code:    "SERVICE_TIMEOUT",
		Message: "service operation timed out",
	}
	ErrServiceOperationFailed = DomainError{
		Code:    "SERVICE_OPERATION_FAILED",
		Message: "service operation failed",
	}
)

// Error classification functions - shared across services

// IsTransientError returns true if the error is likely transient and can be retried
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	var domainErr DomainError
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case "SERVICE_UNAVAILABLE",
			"SERVICE_TIMEOUT",
			"SERVICE_OPERATION_FAILED",
			"EVENT_PUBLISHING_FAILED",
			"REPOSITORY_UNAVAILABLE",
			"RATE_LIMIT_EXCEEDED",
			"QUOTA_EXCEEDED":
			return true
		}
	}

	return false
}

// IsAuthenticationError returns true if the error is related to authentication
func IsAuthenticationError(err error) bool {
	if err == nil {
		return false
	}

	var domainErr DomainError
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case "INVALID_CREDENTIAL",
			"AUTHENTICATION_FAILED",
			"INSUFFICIENT_PERMISSIONS":
			return true
		}
	}

	return false
}

// IsValidationError returns true if the error is related to validation
func IsValidationError(err error) bool {
	if err == nil {
		return false
	}

	var domainErr DomainError
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case "INVALID_CONFIGURATION",
			"INVALID_EVENT_PAYLOAD":
			return true
		}
	}

	return false
}

// IsConfigurationError returns true if the error is related to configuration issues
func IsConfigurationError(err error) bool {
	if err == nil {
		return false
	}

	var domainErr DomainError
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case "MISSING_CONFIGURATION",
			"INVALID_CONFIGURATION":
			return true
		}
	}

	return false
}

// IsRepositoryError returns true if the error is related to database/repository operations
func IsRepositoryError(err error) bool {
	if err == nil {
		return false
	}

	var domainErr DomainError
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case "REPOSITORY_UNAVAILABLE",
			"TRANSACTION_FAILED",
			"CONSTRAINT_VIOLATION":
			return true
		}
	}

	return false
}