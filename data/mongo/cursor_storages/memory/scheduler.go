// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/data/internal/memcleanup"

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

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:             "mongo-cursor-memory-cleanup",
		Description:    "Cleanup expired MongoDB cursors from memory storage",
		Func:           s.cleanupTask.SchedulerFunc(s.runCleanupCycle),
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
	_ = s.cleanupTask.Run(context.Background(), s.runCleanupCycle)
}

// runCleanupCycle performs the actual two-phase cleanup sweep: expired keys
// are collected under the read lock and deleted under the write lock with an
// expiry re-check, so cursors refreshed between the phases survive and
// Store/Load/Delete operations are not blocked for the duration of a full
// scan. Callers must route through cleanupTask so overlapping cycles
// collapse into a single execution.
func (s *Storage) runCleanupCycle(context.Context) error {
	now := time.Now()
	memcleanup.Sweep(&s.mu, s.store, func(e *entry) bool {
		return now.After(e.expiresAt)
	})
	return nil
}
