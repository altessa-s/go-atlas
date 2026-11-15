// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/internal/memcleanup"

	schedtask "github.com/altessa-s/go-horai/task"
)

// registerCleanupTask registers the cleanup cycle with the scheduler.
func (s *Storage) registerCleanupTask() error {
	return memcleanup.Register(context.Background(), s.scheduler, s.cleanupTask, schedtask.Config{
		ID:             "mongo-cursor-memory-cleanup",
		Description:    "Cleanup expired MongoDB cursors from memory storage",
		Schedule:       s.cleanupSchedule,
		Priority:       schedtask.PriorityNormal,
		DisableHistory: true,
	})
}

// RunCleanup removes all expired cursors from storage.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns immediately.
// Thread-safe: Safe for concurrent calls.
func (s *Storage) RunCleanup() {
	_ = s.cleanupTask.Run(context.Background())
}

// runCleanupCycle drops the expired cursors. They sit at the front of the
// eviction order, so the write lock is held for the expired entries only, not
// for a scan of the map. Callers must route through cleanupTask so
// overlapping cycles collapse into a single execution.
func (s *Storage) runCleanupCycle(context.Context) error {
	s.mu.Lock()
	s.evictLocked(time.Now())
	s.mu.Unlock()
	return nil
}
