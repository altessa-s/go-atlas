// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kms

import (
	"errors"
	"fmt"
)

// Base error categories for KMS operations
var (
	// ErrInvalidCredentials indicates that the provided credentials are invalid or incomplete
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidKey indicates that the provided key is invalid or malformed
	ErrInvalidKey = errors.New("invalid key")

	// ErrConfigurationError indicates a configuration problem with the provider
	ErrConfigurationError = errors.New("configuration error")
)

// ValidationError reports a field-level validation failure for a KMS provider.
// It wraps [ErrConfigurationError] via [Unwrap].
type ValidationError struct {
	Provider string // Name of the provider where validation failed
	Field    string // Name of the field that failed validation
	Value    string // The invalid value (may be redacted for sensitive fields)
	Reason   string // Human-readable reason for the failure
}

// Error implements the error interface
func (e ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("validation failed for provider %s field %s (value: %q): %s",
			e.Provider, e.Field, e.Value, e.Reason)
	}
	return fmt.Sprintf("validation failed for provider %s field %s: %s",
		e.Provider, e.Field, e.Reason)
}

// Unwrap returns the underlying error category
func (e ValidationError) Unwrap() error {
	return ErrConfigurationError
}

// CredentialError represents an error with credential handling
type CredentialError struct {
	Provider   string // Name of the provider
	Operation  string // The operation that failed (e.g., "retrieve", "store", "clear")
	Reason     string // Human-readable reason for the failure
	Underlying error  // Optional underlying error
}

// Error implements the error interface
func (e CredentialError) Error() string {
	if e.Underlying != nil {
		return fmt.Sprintf("credential %s failed for provider %s: %s: %v",
			e.Operation, e.Provider, e.Reason, e.Underlying)
	}
	return fmt.Sprintf("credential %s failed for provider %s: %s",
		e.Operation, e.Provider, e.Reason)
}

// Unwrap returns the underlying error
func (e CredentialError) Unwrap() error {
	if e.Underlying != nil {
		return e.Underlying
	}
	return ErrInvalidCredentials
}

// KeyError represents an error with key operations
type KeyError struct {
	Provider   string // Name of the provider
	Operation  string // The operation that failed (e.g., "retrieve", "decrypt", "encrypt")
	KeyID      string // Identifier of the key (may be redacted)
	Reason     string // Human-readable reason for the failure
	Underlying error  // Optional underlying error
}

// Error implements the error interface
func (e KeyError) Error() string {
	if e.Underlying != nil {
		return fmt.Sprintf("key %s failed for provider %s key %q: %s: %v",
			e.Operation, e.Provider, e.KeyID, e.Reason, e.Underlying)
	}
	return fmt.Sprintf("key %s failed for provider %s key %q: %s",
		e.Operation, e.Provider, e.KeyID, e.Reason)
}

// Unwrap returns the underlying error
func (e KeyError) Unwrap() error {
	if e.Underlying != nil {
		return e.Underlying
	}
	return ErrInvalidKey
}

// Helper functions for creating common validation errors

// NewKeyLengthError creates a ValidationError for invalid key length
func NewKeyLengthError(provider string, got, expected int) error {
	return ValidationError{
		Provider: provider,
		Field:    "masterKey",
		Reason:   fmt.Sprintf("invalid length: got %d bytes, expected %d bytes", got, expected),
	}
}

// NewMissingCredentialError creates a ValidationError for missing credential fields
func NewMissingCredentialError(provider, field string) error {
	return ValidationError{
		Provider: provider,
		Field:    field,
		Reason:   "required credential field is missing or empty",
	}
}
