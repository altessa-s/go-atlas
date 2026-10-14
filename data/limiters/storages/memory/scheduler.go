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
// write lock. Allow stamps lastUsed under the lock, so the LRU list is
// ordered by lastUsed and the sweep walks from the tail, stopping at the
// first bucket still in use: O(expired) instead of O(buckets). Callers must
// route through cleanupTask so overlapping cycles collapse into a single
// execution.
func (p *Provider) runCleanupCycle(context.Context) error {
	now := time.Now()
	cutoff := now.Add(-p.options.maxIdleTime)

	p.mu.Lock()
	defer p.mu.Unlock()

	for elem := p.lru.Back(); elem != nil; {
		prev := elem.Prev()
		key, ok := elem.Value.(string)
		if !ok {
			break
		}
		if !p.buckets[key].lastUsed.Before(cutoff) {
			break
		}
		p.removeLocked(key)
		elem = prev
	}
	return nil
}
