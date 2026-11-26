// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/memcleanup"

	schedtask "github.com/altessa-s/go-horai/task"
)

// registerCleanupTask registers the cleanup cycle with the scheduler.
func (s *Storage) registerCleanupTask(schedule string) error {
	return memcleanup.Register(context.Background(), s.scheduler, s.cleanupTask, schedtask.Config{
		ID:             s.options.cleanupTaskID,
		Description:    "Cleanup expired idempotency keys from memory storage",
		Schedule:       schedule,
		Priority:       schedtask.PriorityNormal,
		DisableHistory: true,
	})
}

// RunCleanup removes expired entries from memory using two-phase cleanup.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns immediately.
// Thread-safe: Safe for concurrent calls.
func (s *Storage) RunCleanup() {
	_ = s.cleanupTask.Run(context.Background())
}

// runCleanupCycle performs the actual two-phase cleanup sweep: expired keys
// are collected under the read lock and deleted under the write lock with an
// expiry re-check, so entries refreshed between the phases survive.
// Callers must route through cleanupTask so overlapping cycles collapse
// into a single execution.
func (s *Storage) runCleanupCycle(context.Context) error {
	// Sweep even without a default TTL: AttemptLockWithTTL arms per-call
	// expiries, and entries without one are kept by entry.expired.
	now := s.options.clock()
	memcleanup.Sweep(&s.mu, s.entries, func(e *entry) bool {
		return e.expired(now)
	})
	return nil
}
