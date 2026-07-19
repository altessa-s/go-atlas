// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"errors"
)

// ErrNotFound is returned when a requested secret cannot be found in the storage backend.
// This is a sentinel error that should be returned by all Provider implementations
// when a secret key does not exist or is not accessible.
//
// This error is used throughout the system to distinguish between "secret does not exist"
// and other operational errors like network failures or permission issues. It enables
// proper handling of missing secrets in application logic.
//
// Usage:
//   - Provider implementations should return this error for non-existent secrets
//   - Applications can check for this error using errors.Is(err, ErrNotFound)
//   - Cache misses in the Manager also return this error for consistency
//
// Example:
//
//	secret, err := manager.Value("non_existent_key")
//	if errors.Is(err, secrets.ErrNotFound) {
//		// Handle missing secret gracefully
//		log.Printf("Secret not found, using default configuration")
//		return useDefaultConfig()
//	}
//	if err != nil {
//		// Handle other errors (network, auth, etc.)
//		return fmt.Errorf("failed to retrieve secret: %w", err)
//	}
var ErrNotFound = errors.New("secret not found")

// ErrDecoding is returned when secret value decoding or deserialization fails.
// This error occurs when the stored secret data cannot be converted to the expected
// type T, indicating data corruption, format changes, or encoding mismatches.
//
// Common causes include:
//   - Data corruption in the storage backend
//   - Version incompatibilities between encoding/decoding logic
//   - Type mismatches between stored and expected secret formats
//   - Invalid JSON, YAML, or other structured data formats
//   - Character encoding issues (UTF-8, etc.)
//
// This error is wrapped with additional context by Provider implementations
// to provide detailed information about the decoding failure.
//
// Example:
//
//	secret, err := provider.Value(ctx, "config_json")
//	if errors.Is(err, secrets.ErrDecoding) {
//		// Handle decoding failure
//		log.Printf("Secret data is corrupted or incompatible: %v", err)
//		return handleCorruptedSecret(key)
//	}
var ErrDecoding = errors.New("decoding error")

var ErrEncoding = errors.New("encoding error")

// ErrKeyEncoding is returned when secret key encoding fails before storage operations.
// This error occurs when a secret key cannot be properly encoded using the configured
// key encoder (typically base64), preventing the storage operation from proceeding.
//
// Common causes include:
//   - Invalid characters in the key that cannot be encoded
//   - Key length exceeding encoder limitations
//   - Encoder configuration issues or bugs
//   - Keys containing null bytes or other problematic characters
//
// Provider implementations should return this error when key encoding fails,
// wrapping it with additional context about the specific key and encoding method.
//
// Example:
//
//	err := provider.Save(ctx, "invalid\x00key", "value")
//	if errors.Is(err, secrets.ErrKeyEncoding) {
//		// Handle key encoding issue
//		log.Printf("Invalid key format: %v", err)
//		return validateAndSanitizeKey(key)
//	}
var ErrKeyEncoding = errors.New("key encoding error")

// ErrKeyDecoding is returned when secret key decoding fails during retrieval operations.
// This error occurs when an encoded key from storage cannot be decoded back to its
// original form, typically indicating data corruption or encoding incompatibilities.
//
// Common causes include:
//   - Corrupted key data in the storage backend
//   - Changes in key encoding format between versions
//   - Invalid base64 or other encoded key formats
//   - Character encoding issues in stored key names
//   - Storage backend mangling key names during storage/retrieval
//
// This error indicates a serious issue with the stored key data and may require
// manual intervention to resolve, as the original key cannot be recovered.
//
// Example:
//
//	secrets, err := provider.List(ctx)
//	if errors.Is(err, secrets.ErrKeyDecoding) {
//		// Handle corrupted key data
//		log.Printf("Stored key data is corrupted: %v", err)
//		return reportDataCorruption(err)
//	}
var ErrKeyDecoding = errors.New("key decoding error")

// Common validation errors used across all providers
var (
	// ErrInvalidKey is returned when a secret key is invalid.
	// This includes cases where the key is empty, contains only whitespace,
	// exceeds the maximum allowed length, or contains invalid characters.
	// This unified error simplifies error handling by combining all key validation failures.
	//
	// Common causes include:
	//   - Empty or whitespace-only keys
	//   - Keys exceeding maximum length limits
	//   - Keys containing invalid characters for the storage backend
	//   - Keys with null bytes or other problematic characters
	//
	// Usage:
	//   - All Provider implementations should return this error for invalid keys
	//   - Applications can check for this error using errors.Is(err, ErrInvalidKey)
	//   - Specific validation failure details should be included in the error message
	//
	// Example:
	//
	//	err := provider.Save(ctx, "", "value")
	//	if errors.Is(err, secrets.ErrInvalidKey) {
	//		// Handle invalid key
	//		log.Printf("Invalid key provided: %v", err)
	//		return validateKey(key)
	//	}
	ErrInvalidKey = errors.New("invalid secret key")

	// ErrNilContext is returned when a nil context is provided to a method
	ErrNilContext = errors.New("context cannot be nil")
)

// Graceful shutdown errors
var (
	// ErrShutdownTimeout is returned when a graceful shutdown operation exceeds the specified timeout.
	// This error indicates that not all components could be shut down within the allowed time limit,
	// potentially leaving some background operations or resources in an inconsistent state.
	//
	// When this error occurs, the shutdown process will attempt to force-close remaining operations,
	// but some cleanup may be incomplete. Applications should log this error and consider it
	// a critical issue that may require investigation.
	//
	// Common causes include:
	//   - Long-running background operations that cannot be interrupted cleanly
	//   - Network operations that are slow to respond or hanging
	//   - Large cache clearing operations that take significant time
	//   - Provider-specific cleanup that encounters issues
	//
	// Example:
	//
	//	err := manager.ShutdownWithTimeout(5 * time.Second)
	//	if errors.Is(err, secrets.ErrShutdownTimeout) {
	//		// Handle incomplete shutdown
	//		log.Error("shutdown timed out, some resources may not be cleaned up properly")
	//		// Force exit or escalate to OS-level termination
	//	}
	ErrShutdownTimeout = errors.New("shutdown operation timed out")

	// ErrInvalidShutdownTimeout is returned when an invalid timeout duration is provided
	// to shutdown operations. This includes negative durations, zero durations, or
	// durations that exceed reasonable limits for shutdown operations.
	//
	// Valid timeout ranges are defined by MinShutdownTimeout and MaxShutdownTimeout constants.
	// Applications should use reasonable timeout values that allow for proper cleanup
	// while preventing indefinite hangs.
	//
	// Example:
	//
	//	err := manager.ShutdownWithTimeout(-1 * time.Second)
	//	if errors.Is(err, secrets.ErrInvalidShutdownTimeout) {
	//		// Handle invalid timeout
	//		log.Error("invalid shutdown timeout provided")
	//		return manager.ShutdownWithTimeout(secrets.DefaultShutdownTimeout)
	//	}
	ErrInvalidShutdownTimeout = errors.New("invalid shutdown timeout duration")
)
