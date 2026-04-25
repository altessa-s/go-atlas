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

	// Mark as scheduler-managed
	p.schedulerCleanupRegistered.Store(true)

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:          "limiter-tokenbucket-memory-cleanup",
		Description: "Cleanup expired rate limit buckets from memory storage",
		Func:        func(ctx context.Context) error { p.runCleanupInternal(); return nil },
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
	if p.schedulerCleanupRegistered.Load() {
		return // Managed by scheduler, skip external call
	}
	p.runCleanupInternal()
}

// runCleanupInternal performs the actual cleanup.
// If cleanup is already running, this call returns immediately.
func (p *Provider) runCleanupInternal() {
	// Prevent concurrent execution
	if !p.cleanupRunning.CompareAndSwap(false, true) {
		return // Already running, skip this cycle
	}
	defer p.cleanupRunning.Store(false)

	now := time.Now()
	cutoff := now.Add(-p.options.maxIdleTime)

	p.mu.Lock()
	defer p.mu.Unlock()

	for key, bucket := range p.buckets {
		if bucket.lastUsed.Before(cutoff) {
			delete(p.buckets, key)
		}
	}
}
