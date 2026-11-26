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
func (p *Provider) registerCleanupTask(schedule string) error {
	return memcleanup.Register(context.Background(), p.scheduler, p.cleanupTask, schedtask.Config{
		ID:          "limiter-tokenbucket-memory-cleanup",
		Description: "Cleanup expired rate limit buckets from memory storage",
		Schedule:    schedule,
		Priority:    schedtask.PriorityNormal,
	})
}

// RunCleanup removes idle buckets whose windows have expired.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns immediately.
// Thread-safe: Safe for concurrent calls.
func (p *Provider) RunCleanup() {
	_ = p.cleanupTask.Run(context.Background())
}

// runCleanupCycle removes buckets only after both their idle time and window
// have elapsed. Each shard's LRU is ordered by last use, so the sweep stops
// at the first non-idle bucket but continues past any idle, live windows.
func (p *Provider) runCleanupCycle(context.Context) error {
	now := p.options.clock.Now()
	cutoff := now.Add(-p.options.maxIdleTime)

	for i := range p.shards {
		sh := &p.shards[i]
		sh.mu.Lock()
		for elem := sh.lru.Back(); elem != nil; {
			prev := elem.Prev()
			key, ok := elem.Value.(string)
			if !ok || !sh.buckets[key].lastUsed.Before(cutoff) {
				break
			}
			if sh.buckets[key].expiredAt(now) {
				p.removeLocked(sh, key)
			}
			elem = prev
		}
		sh.mu.Unlock()
	}
	return nil
}
