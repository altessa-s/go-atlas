// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc/connectivity"
)

// stateChangeFn receives connectivity-state updates for one target.
// Multiple subscribers may register for the same target; each per-conn
// transition fans out to all of them.
type stateChangeFn = func(connectivity.State)

// trackedEntry is what [stateTracker] stores per [pooledConnection].
type trackedEntry struct {
	state  connectivity.State
	cancel context.CancelFunc // nil until the watcher goroutine is running
}

// stateTracker watches connectivity state on every conn the pool owns and
// fans changes out to subscribers. Watcher goroutines run only when at least
// one consumer (the pool's optional health helper or an external
// [SubscribeTarget] caller) has opted in. The tracker stays allocated for the
// pool's lifetime so subscribers can register before [ConnectionPool.Start].
type stateTracker struct {
	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	enabled      atomic.Bool

	mu        sync.RWMutex
	perTarget map[string]map[*pooledConnection]*trackedEntry

	subSeq atomic.Int64
	subsMu sync.RWMutex
	subs   map[string]map[int64]stateChangeFn

	// onTargetStateChange is invoked synchronously on the watcher goroutine
	// before subscriber fan-out. Used by the pool's health helper to push
	// status updates into [observability/health.Coordinator]. Set once
	// before [enable] and never again.
	onTargetStateChange func(target string, newState connectivity.State)
}

// newStateTracker constructs a tracker rooted at a fresh background context
// owned by the tracker. The pool's stop function must call [shutdown] to
// release watcher goroutines.
func newStateTracker() *stateTracker {
	ctx, cancel := context.WithCancel(context.Background())
	return &stateTracker{
		parentCtx:    ctx,
		parentCancel: cancel,
		perTarget:    make(map[string]map[*pooledConnection]*trackedEntry),
		subs:         make(map[string]map[int64]stateChangeFn),
	}
}

// enable activates watcher goroutines for every conn already attached and
// any future attach. Idempotent.
func (t *stateTracker) enable() {
	if !t.enabled.CompareAndSwap(false, true) {
		return
	}
	type pending struct {
		target string
		pc     *pooledConnection
		entry  *trackedEntry
	}
	var backfill []pending
	t.mu.RLock()
	for target, conns := range t.perTarget {
		for pc, entry := range conns {
			if entry.cancel == nil {
				backfill = append(backfill, pending{target, pc, entry})
			}
		}
	}
	t.mu.RUnlock()
	for _, p := range backfill {
		t.spawnWatcher(p.target, p.pc, p.entry)
	}
}

// attach records pc as a tracked conn for target. When the tracker is
// enabled it also spawns a watcher goroutine for pc.
func (t *stateTracker) attach(target string, pc *pooledConnection) {
	if pc == nil || pc.conn == nil {
		return
	}
	entry := &trackedEntry{state: connectivity.Idle}
	t.mu.Lock()
	targetMap, ok := t.perTarget[target]
	if !ok {
		targetMap = make(map[*pooledConnection]*trackedEntry)
		t.perTarget[target] = targetMap
	}
	targetMap[pc] = entry
	t.mu.Unlock()

	if t.enabled.Load() {
		t.spawnWatcher(target, pc, entry)
	}
}

// detach cancels the watcher (if any) and removes pc from the tracker.
// Idempotent.
func (t *stateTracker) detach(target string, pc *pooledConnection) {
	if pc == nil {
		return
	}
	t.mu.Lock()
	targetMap, ok := t.perTarget[target]
	if !ok {
		t.mu.Unlock()
		return
	}
	entry, ok := targetMap[pc]
	if !ok {
		t.mu.Unlock()
		return
	}
	cancel := entry.cancel
	delete(targetMap, pc)
	if len(targetMap) == 0 {
		delete(t.perTarget, target)
	}
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// shutdown cancels every watcher and waits for them to exit. Safe to call
// multiple times.
func (t *stateTracker) shutdown() {
	t.parentCancel()
	t.wg.Wait()
}

// spawnWatcher creates the per-conn context and goroutine. Safe under
// concurrent attach/enable: only the first caller for a given entry wins,
// later callers cancel their own context and return.
func (t *stateTracker) spawnWatcher(target string, pc *pooledConnection, entry *trackedEntry) {
	ctx, cancel := context.WithCancel(t.parentCtx)
	t.mu.Lock()
	targetMap, ok := t.perTarget[target]
	if !ok || targetMap[pc] != entry || entry.cancel != nil {
		t.mu.Unlock()
		cancel()
		return
	}
	entry.cancel = cancel
	t.mu.Unlock()

	t.wg.Add(1)
	go t.watch(ctx, target, pc, entry)
}

func (t *stateTracker) watch(ctx context.Context, target string, pc *pooledConnection, entry *trackedEntry) {
	defer t.wg.Done()
	state := pc.conn.GetState()
	t.recordAndFanOut(target, pc, entry, state)
	for pc.conn.WaitForStateChange(ctx, state) {
		state = pc.conn.GetState()
		t.recordAndFanOut(target, pc, entry, state)
	}
}

// recordAndFanOut updates the entry's last-seen state, invokes the
// pool-level callback, and notifies subscribers for this target.
func (t *stateTracker) recordAndFanOut(target string, _ *pooledConnection, entry *trackedEntry, state connectivity.State) {
	t.mu.Lock()
	entry.state = state
	t.mu.Unlock()

	if cb := t.onTargetStateChange; cb != nil {
		cb(target, state)
	}

	t.subsMu.RLock()
	targetSubs := t.subs[target]
	if len(targetSubs) == 0 {
		t.subsMu.RUnlock()
		return
	}
	callbacks := make([]stateChangeFn, 0, len(targetSubs))
	for _, cb := range targetSubs {
		callbacks = append(callbacks, cb)
	}
	t.subsMu.RUnlock()
	for _, cb := range callbacks {
		cb(state)
	}
}

// subscribe registers cb for the given target and implicitly enables the
// tracker. The returned unsubscribe is idempotent.
func (t *stateTracker) subscribe(target string, cb stateChangeFn) func() {
	if cb == nil {
		return func() {}
	}
	id := t.subSeq.Add(1)
	t.subsMu.Lock()
	targetSubs, ok := t.subs[target]
	if !ok {
		targetSubs = make(map[int64]stateChangeFn)
		t.subs[target] = targetSubs
	}
	targetSubs[id] = cb
	t.subsMu.Unlock()

	t.enable()

	return func() {
		t.subsMu.Lock()
		defer t.subsMu.Unlock()
		subs, ok := t.subs[target]
		if !ok {
			return
		}
		delete(subs, id)
		if len(subs) == 0 {
			delete(t.subs, target)
		}
	}
}

// stateForTarget returns the best (most-ready) connectivity state across
// all conns currently tracked for target. Returns [connectivity.Idle] when
// the target is unknown.
func (t *stateTracker) stateForTarget(target string) connectivity.State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	targetMap, ok := t.perTarget[target]
	if !ok || len(targetMap) == 0 {
		return connectivity.Idle
	}
	best := connectivity.Shutdown
	for _, entry := range targetMap {
		if connStateRank(entry.state) > connStateRank(best) {
			best = entry.state
		}
	}
	return best
}

// allTargets returns a snapshot of all tracked target addresses.
func (t *stateTracker) allTargets() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	targets := make([]string, 0, len(t.perTarget))
	for target := range t.perTarget {
		targets = append(targets, target)
	}
	return targets
}

// connStateRank assigns a numerical rank to connectivity states with
// "more ready" being higher. Used to pick the best state across multiple
// connections to the same target.
func connStateRank(s connectivity.State) int {
	//nolint:mnd // ordinal ranks for connectivity.State
	switch s {
	case connectivity.Ready:
		return 5
	case connectivity.Idle:
		return 4
	case connectivity.Connecting:
		return 3
	case connectivity.TransientFailure:
		return 2
	case connectivity.Shutdown:
		return 1
	default:
		return 0
	}
}
