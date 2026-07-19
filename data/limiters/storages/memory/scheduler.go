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
func (p *Provider) registerCleanupTask(schedule string) error {
	if nilcheck.IsNil(p.scheduler) {
		return fmt.Errorf("scheduler not configured")
	}

	if schedule == "" {
		return fmt.Errorf("cleanup schedule must be set")
	}

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:          "limiter-tokenbucket-memory-cleanup",
		Description: "Cleanup expired rate limit buckets from memory storage",
		Func:        p.cleanupTask.SchedulerFunc(p.runCleanupCycle),
		Schedule:    schedule,
		Priority:    corescheduler.TaskPriorityNormal,
	}

	return p.scheduler.Register(ctx, taskCfg)
}

// RunCleanup removes expired and unused buckets.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns immediately.
// Thread-safe: Safe for concurrent calls.
func (p *Provider) RunCleanup() {
	_ = p.cleanupTask.Run(context.Background(), p.runCleanupCycle)
}

// runCleanupCycle performs the actual cleanup, dropping buckets that have
// been idle longer than the configured max idle time. Unlike the TTL-based
// memory storages this sweep is idle-time-based and runs under a single
// write lock. Callers must route through cleanupTask so overlapping cycles
// collapse into a single execution.
func (p *Provider) runCleanupCycle(context.Context) error {
	now := time.Now()
	cutoff := now.Add(-p.options.maxIdleTime)

	p.mu.Lock()
	defer p.mu.Unlock()

	for key, bucket := range p.buckets {
		if bucket.lastUsed.Before(cutoff) {
			delete(p.buckets, key)
		}
	}
	return nil
}
