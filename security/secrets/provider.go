// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"iter"
)

// Provider defines the core interface for accessing and managing secrets from various storage backends.
// It provides a consistent API that abstracts the complexity of different secret storage systems
// such as HashiCorp Vault, Google Cloud Secret Manager, Yandex Cloud Lockbox, and in-memory stores.
//
// The interface supports full CRUD (Create, Read, Update, Delete) operations and is designed
// to be implemented by storage backends while maintaining a unified experience for consuming
// applications. Providers handle the specific details of authentication, network communication,
// encoding/decoding, and error mapping for their respective backends.
//
// Implementation Requirements:
//
// Thread Safety:
// Implementations MUST be thread-safe and support concurrent access from multiple goroutines.
// The Manager will call these methods concurrently during operations like cache warming,
// background updates, and watch notifications.
//
// Error Handling:
// Implementations MUST return standard errors defined in this package (ErrNotFound, ErrDecoding,
// ErrKeyEncoding, etc.) where appropriate. Provider-specific errors should be wrapped with
// additional context while preserving the ability to detect standard error types with errors.Is().
//
// Context Support:
// All methods MUST respect the provided context for cancellation, deadlines, and tracing.
// Long-running operations should periodically check ctx.Err() and return early if canceled.
//
// Retry Logic:
// Providers should implement appropriate retry logic for transient failures (network issues,
// rate limiting, temporary service unavailability). The Manager provides additional retry
// wrapping but providers should handle backend-specific retry requirements.
//
// Generic Type Parameter:
// T represents the type of secret values managed by this provider. Common types include
// string for simple text secrets, []byte for binary data, or custom structs for complex
// configuration objects. The type must be serializable by the configured value decoder.
//
// Example Implementation:
//
//	type MyProvider[T any] struct {
//		client       *MyStorageClient
//		keyEncoder   codec.KeyDecoder
//		valueDecoder codec.ValueDecoder[T]
//	}
//
//	func (p *MyProvider[T]) Name() string {
//		return "my-storage-v1"
//	}
//
//	func (p *MyProvider[T]) Value(ctx context.Context, key string) (*Value[T], error) {
//		// Encode key for storage lookup
//		encodedKey, err := p.keyEncoder.Encode(key)
//		if err != nil {
//			return nil, fmt.Errorf("key encoding failed: %w", ErrKeyEncoding)
//		}
//
//		// Fetch from storage with timeout
//		rawData, version, err := p.client.Get(ctx, encodedKey)
//		if err != nil {
//			if isNotFoundError(err) {
//				return nil, ErrNotFound
//			}
//			return nil, fmt.Errorf("storage get failed: %w", err)
//		}
//
//		// Decode value
//		value, err := p.valueDecoder.Decode(rawData)
//		if err != nil {
//			return nil, fmt.Errorf("value decoding failed: %w", ErrDecoding)
//		}
//
//		return &Value[T]{
//			Key:          key,
//			EncodedKey:   encodedKey,
//			Value:        value,
//			EncodedValue: rawData,
//			Version:      version,
//		}, nil
//	}
type Provider[T any] interface {
	// Name returns a unique identifier for the storage provider.
	// This name is used for logging, metrics, audit trails, and debugging purposes.
	// It should be a short, descriptive string that identifies the backend type
	// (e.g., "vault", "gcp", "lockbox", "memory").
	//
	// The name should remain constant for the lifetime of the provider instance
	// and should not contain sensitive information.
	Name() string

	// List retrieves all available secrets from the storage backend.
	// This method is used during bulk synchronization operations to detect
	// changes, additions, and deletions in the secret store.
	//
	// The method should return all secrets that the authenticated principal
	// has access to, applying any configured filtering (paths, labels, etc.).
	// If no secrets are found or accessible, an empty slice should be returned
	// rather than an error.
	//
	// Parameters:
	//   - ctx: Context for the operation, supporting cancellation and timeouts
	//
	// Returns:
	//   - A slice of all accessible secret values with their metadata
	//   - An error if the operation fails due to authentication, network, or other issues
	List(ctx context.Context) ([]*Value[T], error)

	// Values returns an iterator over all available secrets from the storage backend.
	// This method provides lazy iteration for memory-efficient processing of large
	// secret stores. The iterator yields secrets one at a time without loading all
	// secrets into memory at once.
	//
	// The iterator should respect context cancellation and stop iteration early
	// if the context is canceled. Errors encountered during iteration are yielded
	// as the second value, and iteration should stop after an error.
	//
	// Parameters:
	//   - ctx: Context for the operation, supporting cancellation and timeouts
	//
	// Returns:
	//   - An iterator that yields (value, error) pairs
	//   - Values are yielded with nil error on success
	//   - On error, nil value is yielded with the error, and iteration stops
	//
	// Example usage:
	//
	//	for value, err := range provider.Values(ctx) {
	//	    if err != nil {
	//	        return fmt.Errorf("iteration failed: %w", err)
	//	    }
	//	    process(value)
	//	}
	Values(ctx context.Context) iter.Seq2[*Value[T], error]

	// Value retrieves a specific secret by its key from the storage backend.
	// This method should return the most current version of the secret and
	// include all relevant metadata such as version information.
	//
	// The implementation should apply any necessary key encoding/transformation
	// before querying the backend, and decode the response appropriately.
	//
	// Parameters:
	//   - ctx: Context for the operation, supporting cancellation and timeouts
	//   - key: The secret identifier to retrieve
	//
	// Returns:
	//   - The secret value with metadata if found
	//   - ErrNotFound if the key does not exist
	//   - Other errors for operational failures
	Value(ctx context.Context, key string) (*Value[T], error)

	// Delete removes a secret from the storage backend permanently.
	// This operation should completely remove the secret and all its versions,
	// making it inaccessible for future operations.
	//
	// The method should be idempotent - calling Delete on a non-existent secret
	// should return ErrNotFound rather than a different error type.
	//
	// Parameters:
	//   - ctx: Context for the operation, supporting cancellation and timeouts
	//   - key: The secret identifier to delete
	//
	// Returns:
	//   - nil if the secret was successfully deleted
	//   - ErrNotFound if the secret does not exist
	//   - Other errors for operational failures
	Delete(ctx context.Context, key string) error

	// Save stores or updates a secret value in the storage backend.
	// This operation should create a new secret if the key doesn't exist,
	// or create a new version of an existing secret.
	//
	// The implementation should handle value encoding/serialization and any
	// backend-specific metadata requirements. Version tracking should be
	// managed automatically by the backend where supported.
	//
	// Parameters:
	//   - ctx: Context for the operation, supporting cancellation and timeouts
	//   - key: The secret identifier to store
	//   - value: The secret value to save
	//
	// Returns:
	//   - nil if the secret was successfully saved
	//   - An error if the operation fails
	Save(ctx context.Context, key string, value T) error
}

// Static is an optional interface that providers can implement to indicate
// that their secret set is static and will not change over time. This enables
// important optimizations in the Manager's behavior.
//
// When a provider implements Static and returns true from IsStatic(), the Manager
// will automatically disable background updates, skip periodic refresh operations,
// and perform other optimizations that assume the secret set is immutable.
//
// This interface is particularly useful for:
//   - In-memory providers with fixed secret sets
//   - File-based providers reading from static configuration
//   - Testing scenarios with predetermined secret values
//   - Development environments with embedded secrets
//
// Providers should only implement this interface if their secrets truly never
// change during the application's lifetime. Dynamic providers that occasionally
// have static periods should NOT implement this interface.
//
// Example Implementation:
//
//	type MemoryProvider struct {
//		secrets map[string]string
//	}
//
//	func (p *MemoryProvider) IsStatic() bool {
//		return true // Secrets never change after initialization
//	}
type Static interface {
	// IsStatic returns true if the storage provider contains a static set of secrets
	// that will not change over time. When true, the Manager will disable background
	// updates and other dynamic behavior to optimize performance.
	//
	// This method should consistently return the same value for the lifetime of the
	// provider instance. Changing the return value during runtime may lead to
	// undefined behavior.
	//
	// Returns:
	//   - true: Secret set is immutable, background updates will be disabled
	//   - false: Secret set may change, background updates will be enabled (if requested)
	//
	// Thread Safety:
	//   This method may be called concurrently and should be thread-safe.
	IsStatic() bool
}
