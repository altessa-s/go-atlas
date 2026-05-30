// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
)

// CursorStorage is an interface for storing cursor metadata server-side.
// This provides enhanced security and reduces cursor size by storing metadata
// on the server instead of encoding it in the cursor string sent to the client.
//
// Server-side cursor storage offers several advantages:
//   - Security: Clients cannot tamper with cursor metadata (sort, filters, field names)
//   - Smaller cursors: Client receives only a storage key (e.g., ULID) instead of base64-encoded JSON
//   - Flexibility: Storage can implement TTL, rate limiting, and audit logging
//   - Control: Server controls cursor lifecycle and can invalidate cursors
//
// Implementations must be thread-safe and handle concurrent access.
// TTL and expiration policies are managed by each storage implementation.
//
// Example implementations:
//   - Memory: Fast, for testing or single-instance deployments
//   - Redis: Distributed, with automatic TTL expiration
//   - MongoDB: Persistent, with TTL indexes
type CursorStorage interface {
	// Store saves cursor metadata using the provided storage key.
	// The storage key is generated externally (typically a ULID) and passed to the storage.
	//
	// Storage implementations decide their own TTL policies:
	//   - Fixed TTL (e.g., Redis SETEX with 1 hour)
	//   - Dynamic TTL based on metadata (e.g., complex queries get longer TTL)
	//   - No TTL with manual cleanup (e.g., background jobs)
	//
	// The metadata.CreatedAt field can be used by storage for TTL calculations.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - storageKey: Unique key to store metadata under (e.g., ULID string)
	//   - metadata: Cursor metadata to store
	//
	// Returns:
	//   - error: If storage fails
	//
	// Thread-safety: Must be safe for concurrent calls.
	Store(ctx context.Context, storageKey string, metadata *CursorMetadata) error

	// Load retrieves cursor metadata by storage key.
	// This is called when a client provides a cursor token to continue pagination.
	//
	// Returns:
	//   - *CursorMetadata: The stored metadata
	//   - ErrCursorNotFound: If key doesn't exist or has expired
	//   - error: For other storage errors
	//
	// Thread-safety: Must be safe for concurrent calls.
	Load(ctx context.Context, storageKey string) (*CursorMetadata, error)

	// Delete removes cursor metadata from storage.
	// This is optional and used for explicit cleanup when pagination is complete.
	//
	// Most implementations rely on automatic TTL expiration and can implement
	// this as a no-op. However, explicit deletion can be useful for:
	//   - Immediate resource cleanup
	//   - Security (invalidating cursors after use)
	//   - Storage optimization (removing large cursor sets)
	//
	// Returns:
	//   - error: If deletion fails (implementations may return nil for not-found)
	//
	// Thread-safety: Must be safe for concurrent calls.
	Delete(ctx context.Context, storageKey string) error
}

// Cursor storage errors
var (
	// ErrCursorNotFound indicates the cursor storage key doesn't exist or has expired.
	// This is returned by CursorStorage.Load when a cursor cannot be found.
	// Clients should treat this as an invalid/expired cursor and restart pagination.
	ErrCursorNotFound = errors.New("cursor not found or expired")

	// ErrCursorFilterMismatch indicates the filter has changed between pagination requests.
	// This prevents clients from changing query filters mid-pagination, which would
	// produce inconsistent results. Clients must restart pagination with new filters.
	ErrCursorFilterMismatch = errors.New("cursor filter has changed between requests")

	// ErrCursorFilterUnmarshalable indicates the filter contains a value that
	// json.Marshal cannot encode (NaN/Inf floats, channels, functions, cyclic refs,
	// or a type whose MarshalJSON returned an error). Surfacing this as a typed
	// error prevents silent hash collisions between distinct bad filters and
	// signals that the caller is constructing the filter incorrectly.
	ErrCursorFilterUnmarshalable = errors.New("cursor filter is not JSON-serializable")

	// ErrStorageRequired indicates a server-side cursor was provided but no storage is configured.
	// This occurs when a client sends a ULID-format cursor but ListCursor was called without
	// WithListCursorStorage option.
	ErrStorageRequired = errors.New("cursor storage required for server-side cursor format")

	// ErrInvalidCursorFormat indicates the cursor token format is invalid or unrecognized.
	// Valid formats are:
	//   - ULID (26 characters) for server-side storage (stateful mode)
	//   - Base64-encoded JSON for client-side storage (stateless mode)
	ErrInvalidCursorFormat = errors.New("invalid cursor format")
)
