// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// registerCleanupTask registers cleanup task with scheduler.
func (s *Storage) registerCleanupTask() error {
	if nilcheck.IsNil(s.scheduler) {
		return fmt.Errorf("scheduler not configured")
	}

	if s.cleanupSchedule == "" {
		return fmt.Errorf("cleanup schedule must be set")
	}

	// Mark as scheduler-managed
	s.schedulerCleanupRegistered.Store(true)

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:             "mongo-cursor-memory-cleanup",
		Description:    "Cleanup expired MongoDB cursors from memory storage",
		Func:           func(ctx context.Context) error { s.runCleanupInternal(); return nil },
		Schedule:       s.cleanupSchedule,
		Priority:       corescheduler.TaskPriorityNormal,
		DisableHistory: true,
	}

	return s.scheduler.Register(ctx, taskCfg)
}

// RunCleanup removes all expired cursors from storage.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns immediately.
// Thread-safe: Safe for concurrent calls.
func (s *Storage) RunCleanup() {
	if s.schedulerCleanupRegistered.Load() {
		return // Managed by scheduler, skip external call
	}
	s.runCleanupInternal()
}

// runCleanupInternal performs the actual cleanup.
// If cleanup is already running, this call returns immediately.
//
// This is optimized to minimize write lock duration by:
//  1. Identifying expired keys under read lock
//  2. Deleting them in batch under write lock
//
// This prevents blocking Store/Load/Delete operations for long periods
// when there are many cursors in storage.
func (s *Storage) runCleanupInternal() {
	// Prevent concurrent execution
	if !s.cleanupRunning.CompareAndSwap(false, true) {
		return // Already running, skip this cycle
	}
	defer s.cleanupRunning.Store(false)

	now := time.Now()

	// Phase 1: Identify expired keys (read-only, can run concurrently)
	s.mu.RLock()
	expiredKeys := make([]string, 0, len(s.store)/10) // Pre-allocate assuming ~10% expiration rate
	for key, entry := range s.store {
		if now.After(entry.expiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	s.mu.RUnlock()

	// Phase 2: Delete expired keys (shorter write lock)
	if len(expiredKeys) > 0 {
		s.mu.Lock()
		for _, key := range expiredKeys {
			// Double-check expiration in case entry was updated between phases
			if entry, exists := s.store[key]; exists && now.After(entry.expiresAt) {
				delete(s.store, key)
			}
		}
		s.mu.Unlock()
	}
}
