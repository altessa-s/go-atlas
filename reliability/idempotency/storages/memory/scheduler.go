// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"weak"

	"github.com/altessa-s/go-atlas/internal/memcleanup"

	schedtask "github.com/altessa-s/go-horai/task"
)

// ErrDuplicateCleanupTaskID reports an ID claimed by another memory store
// using the same scheduler.
var ErrDuplicateCleanupTaskID = errors.New("duplicate idempotency memory cleanup task ID")

// The scheduler's registered callback keeps the storage alive. Weak
// references prevent the registry from retaining a discarded scheduler.
var cleanupTasks = struct {
	sync.Mutex
	byID map[string][]weak.Pointer[Storage]
}{byID: make(map[string][]weak.Pointer[Storage])}

type cleanupClaim struct {
	id    string
	store weak.Pointer[Storage]
}

func (c cleanupClaim) release() {
	cleanupTasks.Lock()
	defer cleanupTasks.Unlock()
	stores := slices.DeleteFunc(cleanupTasks.byID[c.id], func(p weak.Pointer[Storage]) bool { return p == c.store })
	if len(stores) == 0 {
		delete(cleanupTasks.byID, c.id)
	} else {
		cleanupTasks.byID[c.id] = stores
	}
}

func (s *Storage) claimCleanupTask() (cleanupClaim, error) {
	if !reflect.ValueOf(s.scheduler).Comparable() {
		return cleanupClaim{}, errors.New("idempotency memory cleanup scheduler must have comparable identity")
	}
	claim := cleanupClaim{id: s.options.cleanupTaskID, store: weak.Make(s)}
	cleanupTasks.Lock()
	defer cleanupTasks.Unlock()
	for _, p := range cleanupTasks.byID[claim.id] {
		if other := p.Value(); other != nil && other.scheduler == s.scheduler {
			return cleanupClaim{}, ErrDuplicateCleanupTaskID
		}
	}
	cleanupTasks.byID[claim.id] = append(cleanupTasks.byID[claim.id], claim.store)
	return claim, nil
}

// registerCleanupTask registers the cleanup cycle with the scheduler.
func (s *Storage) registerCleanupTask(schedule string) error {
	claim, err := s.claimCleanupTask()
	if err != nil {
		return err
	}
	if err := memcleanup.Register(context.Background(), s.scheduler, s.cleanupTask, schedtask.Config{
		ID:             s.options.cleanupTaskID,
		Description:    "Cleanup expired idempotency keys from memory storage",
		Schedule:       schedule,
		Priority:       schedtask.PriorityNormal,
		DisableHistory: true,
	}); err != nil {
		claim.release()
		return err
	}
	runtime.AddCleanup(s, cleanupClaim.release, claim)
	return nil
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
	now := s.options.clock.Now()
	memcleanup.Sweep(&s.mu, s.entries, func(e *entry) bool {
		return e.expired(now)
	})
	return nil
}
