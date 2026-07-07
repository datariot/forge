// Package forgeerrors provides domain-specific error handling patterns for Forge microservices.
//
// The forgeerrors package implements structured error handling with error codes, context,
// and error classification. It provides common error patterns that can be shared
// across microservices and enables consistent error handling and reporting.
//
// # Basic Usage
//
// Use the predefined domain errors and customize with context:
//
//	if db.Ping() != nil {
//		return forgeerrors.ErrRepositoryUnavailable.WithMessage("database connection failed")
//	}
//
//	// With cause
//	if err := validateUser(user); err != nil {
//		return forgeerrors.ErrInvalidConfiguration.WithCause(err)
//	}
//
// # Error Classification
//
// The package provides classification functions for error handling:
//
//	if forgeerrors.IsTransientError(err) {
//		// Retry the operation
//		time.Sleep(backoff)
//		return retryOperation()
//	}
//
//	if forgeerrors.IsAuthenticationError(err) {
//		// Return 401 Unauthorized
//		return handleAuthError(err)
//	}
//
// # Custom Domain Errors
//
// Create service-specific errors using the DomainError pattern:
//
//	var ErrUserNotFound = forgeerrors.DomainError{
//		Code:    "USER_NOT_FOUND",
//		Message: "user not found",
//	}
//
//	// Usage
//	return ErrUserNotFound.WithMessage("user %s not found", userID)
package forgeerrors

import (
	"errors"
	"fmt"
)

// DomainError represents a domain-specific error with additional context.
// DomainError implements the error interface and provides structured error
// information including error codes, human-readable messages, and optional
// underlying causes.
//
// DomainErrors are designed to be:
//   - Serializable to JSON for API responses
//   - Wrappable using Go's error wrapping patterns
//   - Classifiable using the provided classification functions
//   - Contextual with additional information via WithMessage and WithCause
//
// Example:
//
//	err := ErrRepositoryUnavailable.
//		WithMessage("failed to connect to user database").
//		WithCause(sqlErr)
//
//	// Can be unwrapped
//	if errors.Is(err, sqlErr) {
//		// Handle specific SQL error
//	}
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
func (e DomainError) WithMessage(format string, args ...any) DomainError {
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

// Is implements the errors.Is interface by comparing error codes.
// Two DomainErrors are considered equal if they share the same Code,
// regardless of message or cause. This enables errors.Is(err, ErrRepositoryUnavailable)
// to match variants created via WithMessage() or WithCause().
func (e DomainError) Is(target error) bool {
	var t DomainError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
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

// IsTransientError returns true if the error is likely transient and can be retried.
// Transient errors include network timeouts, service unavailability, rate limiting,
// and other temporary conditions that might resolve on retry.
//
// Use this for implementing retry logic:
//
//	if forgeerrors.IsTransientError(err) {
//		return backoff.Retry(operation, backoff.NewExponentialBackOff())
//	}
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
