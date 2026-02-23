// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sync"

	"github.com/altessa-s/go-atlas/security/secrets"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

// validateMemoryValues validates the initial values map
func validateMemoryValues[T any](values map[string]T) error {
	if len(values) == 0 {
		return nil
	}

	// Validate all keys in the map
	for key := range values {
		if err := secrets.ValidateSecretKey(key); err != nil {
			return fmt.Errorf("key '%s' invalid: %w", key, err)
		}
	}

	return nil
}

// Verify that Storage implements the secrets.Provider interface
var _ secrets.Provider[secrets.Value[any]] = (*Storage[secrets.Value[any]])(nil)

// Verify that Storage implements the secrets.Static interface
var _ secrets.Static = (*Storage[any])(nil)

// Storage implements the secrets.Provider interface for in-memory secret storage.
// It provides fast access to a fixed set of secrets loaded during initialization.
// All secrets are stored in memory and served without any external calls.
type Storage[T any] struct {
	mx sync.RWMutex
	// values holds all the secrets in memory, mapped by their keys
	values map[string]*secrets.Value[T]
}

// New creates a new memory storage provider with the given secrets.
// All provided secrets are loaded into memory and remain static throughout
// the application lifetime.
//
// Returns a new Storage instance with all secrets preloaded.
func New[T any](values map[string]T) (*Storage[T], error) {
	// Validate input parameters
	if err := validateMemoryValues(values); err != nil {
		return nil, fmt.Errorf("values invalid: %w", err)
	}

	s := &Storage[T]{
		values: make(map[string]*secrets.Value[T], len(values)),
	}

	for k, v := range values {
		s.values[k] = secrets.NewValue(k, v, nil, "")
	}

	return s, nil
}

// Name returns the name of the storage provider, which is "memory".
func (s *Storage[T]) Name() string { return "memory" }

// Delete removes a secret from memory storage.
// This operation removes the secret from the in-memory map.
//
// Returns secrets.ErrNotFound if the key doesn't exist.
func (s *Storage[T]) Delete(_ context.Context, key string) error {
	if err := secrets.ValidateSecretKey(key); err != nil {
		return fmt.Errorf("secret key invalid: %w", err)
	}

	s.mx.Lock()
	defer s.mx.Unlock()
	if _, ok := s.values[key]; ok {
		delete(s.values, key)
		return nil
	}
	return secrets.ErrNotFound
}

// Save stores or updates a secret in memory storage.
// This operation adds or updates the secret in the in-memory map.
//
// Returns nil if the save was successful, or an error.
func (s *Storage[T]) Save(_ context.Context, key string, value T) error {
	if err := secrets.ValidateSecretKey(key); err != nil {
		return fmt.Errorf("secret key invalid: %w", err)
	}

	s.mx.Lock()
	defer s.mx.Unlock()

	s.values[key] = secrets.NewValue(key, value, nil, "")
	return nil
}

// IsStatic returns true, indicating that this storage provider contains
// a static set of secrets that will not change over time. This allows
// consumers to avoid unnecessary refresh operations.
func (s *Storage[T]) IsStatic() bool { return true }

// List returns all secrets stored in the memory provider.
// Since all secrets are stored in memory, this operation is very fast
// and does not require any external calls.
//
// Returns a slice of all secrets, or an error.
func (s *Storage[T]) List(_ context.Context) ([]*secrets.Value[T], error) {
	s.mx.RLock()
	defer s.mx.RUnlock()

	if len(s.values) == 0 {
		return []*secrets.Value[T]{}, nil
	}

	// Get a pooled slice buffer with capacity hint to reduce allocations
	valuesPtr := secrets.GetValueSliceWithCapacity[T](len(s.values))
	defer secrets.PutValueSlice(valuesPtr) // Return buffer to pool when done

	// Copy values using iterator
	values := slices.AppendSeq(*valuesPtr, maps.Values(s.values))

	// Create a copy of the results so the original buffer can be returned to pool
	result := make([]*secrets.Value[T], len(values))
	copy(result, values)

	return result, nil
}

// Value retrieves a specific secret by key from memory storage.
// This operation is performed entirely in memory and is very fast.
//
// Returns the secret value if found, or secrets.ErrNotFound if the key doesn't exist.
func (s *Storage[T]) Value(_ context.Context, key string) (*secrets.Value[T], error) {
	if err := secrets.ValidateSecretKey(key); err != nil {
		return nil, fmt.Errorf("secret key invalid: %w", err)
	}

	s.mx.RLock()
	defer s.mx.RUnlock()

	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return nil, secrets.ErrNotFound
}

// Values returns an iterator over all secrets stored in the memory provider.
// This method provides lazy iteration for memory-efficient processing.
// Since all secrets are stored in memory, this operation is very fast.
//
// Returns an iterator yielding (value, error) pairs.
func (s *Storage[T]) Values(ctx context.Context) iter.Seq2[*secrets.Value[T], error] {
	return func(yield func(*secrets.Value[T], error) bool) {
		ctx = corecontext.OrBackground(ctx)

		s.mx.RLock()
		defer s.mx.RUnlock()

		for _, value := range s.values {
			// Check context cancellation
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			if !yield(value, nil) {
				return
			}
		}
	}
}

// CheckConnection verifies the memory provider's operational status.
// This method implements the ProviderHealthChecker interface. Since the memory
// provider doesn't have external dependencies, this check primarily validates
// internal consistency and basic operational readiness.
//
// Parameters:
//   - ctx: context for the health check operation, supports cancellation and timeouts
//
// Returns nil if the provider is healthy, or an error describing the issue.
func (s *Storage[T]) CheckConnection(_ context.Context) error {
	return nil
}
