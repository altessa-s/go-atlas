// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	vaultApi "github.com/hashicorp/vault/api"
)

// Predefined error types for authentication operations.
var (
	// ErrUnauthorized is returned when authentication fails due to invalid or missing credentials.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrAuthInitializeFailed is returned when the auth method fails to initialize.
	ErrAuthInitializeFailed = errors.New("unable to initialize auth method")
	// ErrEmptyResponse is returned when Vault returns an empty response.
	ErrEmptyResponse = errors.New("empty response from Vault")
	// ErrInvalidCredentials is returned when credentials are rejected by Vault.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrPermissionDenied is returned when the token lacks required permissions.
	ErrPermissionDenied = errors.New("permission denied")
)

// AuthError represents a structured authentication error with additional context.
// Use NewAuthError or WrapAuthError to create instances.
type AuthError struct {
	Method  string // Authentication method name
	Reason  string // Human-readable reason
	Code    int    // HTTP status code if applicable
	Wrapped error  // Underlying error
}

// Error implements the error interface.
func (e *AuthError) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("auth method %s failed: %s (code: %d): %v",
			e.Method, e.Reason, e.Code, e.Wrapped)
	}
	return fmt.Sprintf("auth method %s failed: %s (code: %d)",
		e.Method, e.Reason, e.Code)
}

// Unwrap returns the underlying error for error unwrapping.
func (e *AuthError) Unwrap() error {
	return e.Wrapped
}

// Is implements error comparison for errors.Is().
func (e *AuthError) Is(target error) bool {
	if e.Wrapped != nil && errors.Is(e.Wrapped, target) {
		return true
	}

	switch target {
	case ErrUnauthorized:
		return e.Code == http.StatusForbidden || e.Code == http.StatusUnauthorized
	case ErrPermissionDenied:
		return e.Code == http.StatusForbidden
	case ErrInvalidCredentials:
		return e.Code == http.StatusUnauthorized
	}

	return false
}

// NewAuthError creates a new AuthError with the given parameters.
//
// Example:
//
//	err := NewAuthError("approle", "invalid role ID", 401, nil)
func NewAuthError(method, reason string, code int, wrapped error) *AuthError {
	return &AuthError{
		Method:  method,
		Reason:  reason,
		Code:    code,
		Wrapped: wrapped,
	}
}

// WrapAuthError wraps an existing error with authentication context.
// Returns nil if err is nil. Does not double-wrap existing AuthErrors.
//
// Example:
//
//	if err != nil {
//		return WrapAuthError("approle", err)
//	}
func WrapAuthError(method string, err error) error {
	if err == nil {
		return nil
	}

	// If it's already an AuthError, don't double-wrap
	if _, ok := coreerrs.AsType[*AuthError](err); ok { //nolint:errcheck // only checking ok
		return err
	}

	// Check if it's a Vault API response error
	if vaultErr, ok := coreerrs.AsType[*vaultApi.ResponseError](err); ok {
		reason := "authentication failed"
		if len(vaultErr.Errors) > 0 {
			// Sanitize error message to avoid leaking sensitive information
			reason = sanitizeErrorMessage(vaultErr.Errors[0])
		}

		return NewAuthError(method, reason, vaultErr.StatusCode, err)
	}

	// Generic error wrapping
	return NewAuthError(method, "authentication error", 0, err)
}

// IsAuthenticationError checks if an error indicates an authentication problem
// that should not be retried (invalid credentials, permission denied, etc.).
//
// Example:
//
//	if IsAuthenticationError(err) {
//		log.Fatal("invalid credentials, not retrying")
//	}
func IsAuthenticationError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific error types
	if errors.Is(err, ErrUnauthorized) ||
		errors.Is(err, ErrInvalidCredentials) ||
		errors.Is(err, ErrPermissionDenied) {
		return true
	}

	// Check Vault API response errors
	if vaultErr, ok := coreerrs.AsType[*vaultApi.ResponseError](err); ok {
		switch vaultErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return true
		}

		// Check specific error messages
		for _, errMsg := range vaultErr.Errors {
			switch errMsg {
			case "invalid secret id", "invalid role ID", "invalid username or password":
				return true
			}
		}
	}

	return false
}

// IsRetryableError checks if an error is retryable (network issues, server errors, etc.).
// Authentication errors are not considered retryable.
//
// Example:
//
//	if IsRetryableError(err) {
//		time.Sleep(backoff)
//		continue // retry
//	}
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Authentication errors are not retryable
	if IsAuthenticationError(err) {
		return false
	}

	// Check Vault API response errors
	if vaultErr, ok := coreerrs.AsType[*vaultApi.ResponseError](err); ok {
		// 5xx errors are generally retryable
		if vaultErr.StatusCode >= 500 && vaultErr.StatusCode < 600 {
			return true
		}
		// 429 (rate limit) is retryable
		if vaultErr.StatusCode == http.StatusTooManyRequests {
			return true
		}
		return false
	}

	// Network and other infrastructure errors are generally retryable
	return true
}

// sanitizeErrorMessage removes sensitive information from error messages
// while preserving useful debugging information.
func sanitizeErrorMessage(errorMsg string) string {
	// Convert to lowercase for case-insensitive matching
	lowerMsg := corestrings.InternLowerString(errorMsg)

	// Define safe error messages that don't contain sensitive data
	safeErrors := map[string]string{
		"invalid secret id":            "invalid secret id",
		"invalid role id":              "invalid role id",
		"invalid username or password": "invalid username or password",
		"permission denied":            "permission denied",
		"unauthorized":                 "unauthorized",
		"forbidden":                    "forbidden",
		"authentication failed":        "authentication failed",
		"invalid request":              "invalid request",
		"missing required field":       "missing required field",
		"bad request":                  "bad request",
		"internal server error":        "internal server error",
		"service unavailable":          "service unavailable",
		"timeout":                      "timeout",
		"connection refused":           "connection refused",
		"network error":                "network error",
	}

	// Check if the error message matches any known safe patterns
	for pattern, safeMsg := range safeErrors {
		if strings.Contains(lowerMsg, pattern) {
			return safeMsg
		}
	}

	// For unknown error messages, return a generic safe message
	// to avoid potentially leaking sensitive information
	return "authentication failed"
}
