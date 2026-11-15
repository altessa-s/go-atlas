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

// RunCleanup removes expired and unused buckets.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns immediately.
// Thread-safe: Safe for concurrent calls.
func (p *Provider) RunCleanup() {
	_ = p.cleanupTask.Run(context.Background())
}

// runCleanupCycle performs the actual cleanup, dropping buckets that have
// been idle longer than the configured max idle time. Unlike the TTL-based
// memory storages this sweep is idle-time-based. It locks one shard at a
// time, so hits on other shards keep running. Allow stamps lastUsed under
// the shard lock, so each shard's LRU list is ordered by lastUsed and the
// sweep walks each tail, stopping at the first bucket still in use:
// O(shards + expired) instead of O(buckets). Callers must route through
// cleanupTask so overlapping cycles collapse into a single execution.
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
			p.removeLocked(sh, key)
			elem = prev
		}
		sh.mu.Unlock()
	}
	return nil
}
