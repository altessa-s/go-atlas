// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"time"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// registerCleanupTask registers cleanup task with scheduler.
func (s *Storage) registerCleanupTask(schedule string) error {
	if s.scheduler == nil {
		return fmt.Errorf("scheduler not configured")
	}

	if schedule == "" {
		return fmt.Errorf("cleanup schedule must be set")
	}

	// Mark as scheduler-managed
	s.schedulerCleanupRegistered.Store(true)

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:             "idempotency-memory-cleanup",
		Description:    "Cleanup expired idempotency keys from memory storage",
		Func:           func(ctx context.Context) error { s.runCleanupInternal(); return nil },
		Schedule:       schedule,
		Priority:       corescheduler.TaskPriorityNormal,
		DisableHistory: true,
	}

	return s.scheduler.Register(ctx, taskCfg)
}

// RunCleanup removes expired entries from memory using two-phase cleanup.
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
func (s *Storage) runCleanupInternal() {
	// Prevent concurrent execution
	if !s.cleanupRunning.CompareAndSwap(false, true) {
		return // Already running, skip this cycle
	}
	defer s.cleanupRunning.Store(false)

	if s.options.ttl <= 0 {
		return // No TTL, nothing to clean
	}

	now := time.Now()
	var keysToDelete []string

	// Phase 1: Identify expired keys under read lock
	s.mu.RLock()
	for key, e := range s.entries {
		if now.After(e.expiresAt) {
			keysToDelete = append(keysToDelete, key)
		}
	}
	s.mu.RUnlock()

	// Phase 2: Delete in batch under write lock
	if len(keysToDelete) > 0 {
		s.mu.Lock()
		for _, key := range keysToDelete {
			delete(s.entries, key)
		}
		s.mu.Unlock()
	}
}
