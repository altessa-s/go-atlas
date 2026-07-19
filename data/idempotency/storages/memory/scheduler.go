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
func (s *Storage) registerCleanupTask(schedule string) error {
	if nilcheck.IsNil(s.scheduler) {
		return fmt.Errorf("scheduler not configured")
	}

	if schedule == "" {
		return fmt.Errorf("cleanup schedule must be set")
	}

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:             "idempotency-memory-cleanup",
		Description:    "Cleanup expired idempotency keys from memory storage",
		Func:           s.cleanupTask.SchedulerFunc(s.runCleanupCycle),
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
	_ = s.cleanupTask.Run(context.Background(), s.runCleanupCycle)
}

// runCleanupCycle performs the actual two-phase cleanup sweep: expired keys
// are collected under the read lock and deleted under the write lock with an
// expiry re-check, so entries refreshed between the phases survive.
// Callers must route through cleanupTask so overlapping cycles collapse
// into a single execution.
func (s *Storage) runCleanupCycle(context.Context) error {
	if s.options.ttl <= 0 {
		return nil // No TTL, nothing to clean
	}

	now := time.Now()
	memcleanup.Sweep(&s.mu, s.entries, func(e *entry) bool {
		return now.After(e.expiresAt)
	})
	return nil
}
