// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/mongo"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Storage is an in-memory implementation of mongotools.CursorStorage.
// This implementation is primarily intended for testing and development,
// but can also be used in single-instance deployments where cursor sharing
// across instances is not required.
//
// Features:
//   - Thread-safe: Safe for concurrent access
//   - Configurable TTL: Each cursor expires after the configured duration
//   - ULID keys: Lexicographically sortable, time-ordered storage keys
//
// Limitations:
//   - Not distributed: Cursors are not shared across multiple instances
//   - Not persistent: Cursors are lost on restart
//   - Memory usage: Grows with number of active cursors
//
// Example usage:
//
//	storage := memory.New(1 * time.Hour)
//	defer storage.Close()
//
//	// With scheduler for automatic cleanup
//	storage := memory.New(1 * time.Hour,
//	    memory.WithScheduler(scheduler),
//	    memory.WithCleanupSchedule("@every 5m"),
//	)
//
//	result, err := mongotools.ListCursor(ctx, collection,
//	    mongotools.WithListCursorStorage(storage),
//	    mongotools.WithListCursorLimit(50),
//	)
type Storage struct {
	store           map[string]*entry
	mu              sync.RWMutex
	ttl             time.Duration
	scheduler       corescheduler.TaskRegistrar
	cleanupSchedule string
	cleanupTask     corescheduler.ManagedTask // Guards RunCleanup and marks scheduler management.
}

// entry represents a stored cursor with expiration time.
type entry struct {
	metadata  *mongo.CursorMetadata
	expiresAt time.Time
}

const (
	// DefaultTTL is the default time-to-live for cursors (1 hour).
	// This is a reasonable default for most pagination use cases.
	DefaultTTL = 1 * time.Hour
)

// Option configures Storage creation.
type Option func(*Storage)

// WithScheduler sets the scheduler for cleanup task registration.
// If scheduler is provided, cleanup task will be registered instead of using internal ticker.
func WithScheduler(s corescheduler.TaskRegistrar) Option {
	return func(st *Storage) {
		st.scheduler = s
	}
}

// WithCleanupSchedule sets the cron schedule for cleanup task registration.
func WithCleanupSchedule(schedule string) Option {
	return func(st *Storage) {
		st.cleanupSchedule = schedule
	}
}

// New creates a new in-memory cursor storage with the specified TTL.
//
// If scheduler is provided and cleanupSchedule is set, cleanup task will be registered.
// Otherwise, call RunCleanup() manually or register it externally.
//
// Parameters:
//   - ttl: Time-to-live for cursors. If <= 0, uses DefaultTTL (1 hour)
//   - opts: Optional configuration options
//
// Returns:
//   - *Storage: Ready to use storage instance
func New(ttl time.Duration, opts ...Option) *Storage {
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	s := &Storage{
		store: make(map[string]*entry),
		ttl:   ttl,
	}

	// Apply options
	for _, opt := range opts {
		opt(s)
	}

	// Register cleanup task with scheduler if provided
	if s.scheduler != nil && s.cleanupSchedule != "" {
		_ = s.registerCleanupTask() //nolint:errcheck // task registration is optional
	}

	return s
}

// Store saves cursor metadata using the provided storage key.
// The storage key is generated externally (typically a ULID) and passed to the storage.
//
// Thread-safe: Safe for concurrent calls.
func (s *Storage) Store(ctx context.Context, key string, metadata *mongo.CursorMetadata) error {
	// Calculate expiration time
	expiresAt := time.Now().Add(s.ttl)

	// Store with lock
	s.mu.Lock()
	s.store[key] = &entry{
		metadata:  metadata,
		expiresAt: expiresAt,
	}
	s.mu.Unlock()

	return nil
}

// Load retrieves cursor metadata by storage key.
// Returns mongotools.ErrCursorNotFound if key doesn't exist or has expired.
//
// Thread-safe: Safe for concurrent calls.
func (s *Storage) Load(ctx context.Context, key string) (*mongo.CursorMetadata, error) {
	// Check existence and expiration under read lock to prevent race condition
	// with cleanup goroutine
	s.mu.RLock()
	entry, exists := s.store[key]
	expired := exists && time.Now().After(entry.expiresAt)
	s.mu.RUnlock()

	if !exists || expired {
		// Remove expired entry (defensive: cleanup might have already removed it)
		if expired {
			s.mu.Lock()
			delete(s.store, key)
			s.mu.Unlock()
		}
		return nil, mongo.ErrCursorNotFound
	}

	return entry.metadata, nil
}

// Delete removes cursor metadata from storage.
// Returns nil if key doesn't exist (idempotent).
//
// Thread-safe: Safe for concurrent calls.
func (s *Storage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	delete(s.store, key)
	s.mu.Unlock()
	return nil
}

// Len returns the number of cursors currently stored.
// Useful for testing and monitoring.
//
// Thread-safe: Safe for concurrent calls.
func (s *Storage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.store)
}

// Close releases resources.
// No-op since there are no background goroutines to stop.
func (s *Storage) Close() {
	// No-op: no background goroutines to stop
}
