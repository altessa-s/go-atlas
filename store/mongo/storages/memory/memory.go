// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/store/mongo"

	schedtask "github.com/altessa-s/go-horai/task"
)

// Storage is an in-memory implementation of mongotools.Storage.
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
//   - Memory usage: Grows with number of active cursors, up to the entry caps
//   - Tenant isolation: The per-subject cap covers only cursors bound to a
//     subject; unbound cursors of all callers share the global cap
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
	store map[string]*entry
	// order lists the stored entries oldest first. The TTL is constant, so it
	// is also the expiry order; Store evicts from its front. An entry leaves
	// the list the moment it leaves the map, so both always have equal length.
	order *list.List
	// subjects lists the entries of each subject oldest first, for the
	// per-subject cap. A subject without entries has no list.
	subjects             map[string]*list.List
	mu                   sync.RWMutex
	ttl                  time.Duration
	maxEntries           int
	maxEntriesPerSubject int
	scheduler            schedtask.Registrar
	cleanupSchedule      string
	cleanupTask          *cycle.Guard // Guards RunCleanup and marks scheduler management.
}

// entry represents a stored cursor with expiration time.
type entry struct {
	key       string
	metadata  *mongo.CursorMetadata
	expiresAt time.Time
	// subject is the cursor's subject at Store time; the caller keeps the
	// metadata pointer and may change it afterwards.
	subject string
	// elem is the entry's position in Storage.order.
	elem *list.Element
	// subjectElem is the entry's position in its subject's list, nil when the
	// entry is not counted against the per-subject cap.
	subjectElem *list.Element
}

const (
	// DefaultTTL is the default time-to-live for cursors (1 hour).
	// This is a reasonable default for most pagination use cases.
	DefaultTTL = 1 * time.Hour

	// DefaultMaxEntries is the default cap on the number of stored cursors.
	DefaultMaxEntries = 100_000

	// DefaultMaxEntriesPerSubject is the default cap on the number of stored
	// cursors bound to one subject under DefaultMaxEntries, and the upper
	// bound of the default under any other WithMaxEntries: a tenth of the
	// global cap, at least 1.
	DefaultMaxEntriesPerSubject = DefaultMaxEntries / subjectCapDivisor

	// subjectCapDivisor is the share of the global cap one subject gets by
	// default: maxEntries / subjectCapDivisor.
	subjectCapDivisor = 10
)

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

	o := newOptions(opts...)
	s := &Storage{
		store:                make(map[string]*entry),
		order:                list.New(),
		subjects:             make(map[string]*list.List),
		ttl:                  ttl,
		maxEntries:           o.maxEntries,
		maxEntriesPerSubject: o.subjectCap(),
		scheduler:            o.scheduler,
		cleanupSchedule:      o.cleanupSchedule,
	}
	s.cleanupTask = cycle.New(s.runCleanupCycle)

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
	e := &entry{key: key, metadata: metadata}
	if metadata != nil {
		e.subject = metadata.Subject
	}

	s.mu.Lock()
	// The clock is read under the lock so the order of the list is the order
	// of expiry.
	now := time.Now()
	e.expiresAt = now.Add(s.ttl)
	if old, ok := s.store[key]; ok {
		s.removeLocked(old)
	}
	s.insertLocked(e)
	s.evictLocked(now)
	s.mu.Unlock()

	return nil
}

// insertLocked adds e as the newest entry. When its subject is then over
// maxEntriesPerSubject, the oldest entry of that subject is evicted, so a
// subject at its cap never costs another one a cursor. Callers hold the write
// lock and have removed any entry stored under the same key.
func (s *Storage) insertLocked(e *entry) {
	s.store[e.key] = e
	e.elem = s.order.PushBack(e)

	if s.maxEntriesPerSubject == 0 || e.subject == "" {
		return
	}
	bound := s.subjects[e.subject]
	if bound == nil {
		bound = list.New()
		s.subjects[e.subject] = bound
	}
	e.subjectElem = bound.PushBack(e)
	if bound.Len() > s.maxEntriesPerSubject {
		s.removeLocked(bound.Front().Value.(*entry)) //nolint:errcheck,forcetypeassert // the lists hold *entry only
	}
}

// removeLocked drops e from the map and from both orders in constant time, so
// no stale position is left behind to count toward a cap. Callers hold the
// write lock; e must be the entry currently stored under its key.
func (s *Storage) removeLocked(e *entry) {
	delete(s.store, e.key)
	s.order.Remove(e.elem)
	if e.subjectElem == nil {
		return
	}
	bound := s.subjects[e.subject]
	bound.Remove(e.subjectElem)
	if bound.Len() == 0 {
		delete(s.subjects, e.subject)
	}
}

// evictLocked drops the oldest entries while they are expired or the storage
// holds more than maxEntries live entries. Callers hold the write lock.
func (s *Storage) evictLocked(now time.Time) {
	for front := s.order.Front(); front != nil; front = s.order.Front() {
		e := front.Value.(*entry) //nolint:errcheck,forcetypeassert // the list holds *entry only
		if !now.After(e.expiresAt) && len(s.store) <= s.maxEntries {
			return
		}
		s.removeLocked(e)
	}
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
			s.deleteIfSame(key, entry)
		}
		return nil, mongo.ErrCursorNotFound
	}

	return entry.metadata, nil
}

// deleteIfSame removes key only while it still maps to observed. A Store that
// replaced the entry between Load's read and write locks keeps its fresh
// entry.
func (s *Storage) deleteIfSame(key string, observed *entry) {
	s.mu.Lock()
	if s.store[key] == observed {
		s.removeLocked(observed)
	}
	s.mu.Unlock()
}

// Delete removes cursor metadata from storage.
// Returns nil if key doesn't exist (idempotent).
//
// Thread-safe: Safe for concurrent calls.
func (s *Storage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	if e, ok := s.store[key]; ok {
		s.removeLocked(e)
	}
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
