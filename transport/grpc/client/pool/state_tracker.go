// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc/connectivity"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
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

// subscriberQueueSize bounds the notifications waiting for one subscriber. A
// subscriber that falls behind loses its oldest states and still receives the
// latest one, which is the only state a health consumer acts on.
const subscriberQueueSize = 16

// subscriber delivers a target's state changes to one callback on its own
// goroutine, so a callback may call [stateTracker.shutdown] or the pool's stop
// function without waiting for itself. Notifications of one subscriber stay in
// order; a slow subscriber never delays watchers or other subscribers.
type subscriber struct {
	cb   stateChangeFn
	wake chan struct{}
	done chan struct{}

	mu    sync.Mutex
	queue []connectivity.State

	closeOnce sync.Once
}

func newSubscriber(cb stateChangeFn) *subscriber {
	s := &subscriber{cb: cb, wake: make(chan struct{}, 1), done: make(chan struct{})}
	// The goroutine lives until close and is deliberately outside the
	// tracker's watcher wait group.
	go s.run()
	return s
}

// enqueue records state for delivery. A repeat of the newest queued state is
// dropped and a full queue discards its oldest entry.
func (s *subscriber) enqueue(state connectivity.State) {
	s.mu.Lock()
	switch {
	case len(s.queue) > 0 && s.queue[len(s.queue)-1] == state:
	case len(s.queue) >= subscriberQueueSize:
		s.queue = append(s.queue[1:], state)
	default:
		s.queue = append(s.queue, state)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *subscriber) next() (connectivity.State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return connectivity.Idle, false
	}
	state := s.queue[0]
	s.queue = s.queue[1:]
	return state, true
}

func (s *subscriber) run() {
	for {
		select {
		case <-s.done:
			return
		case <-s.wake:
		}
		for {
			state, ok := s.next()
			if !ok {
				break
			}
			select {
			case <-s.done:
				return
			default:
			}
			s.cb(state)
		}
	}
}

// close stops delivery without waiting for a callback in progress.
func (s *subscriber) close() { s.closeOnce.Do(func() { close(s.done) }) }

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

	closed    bool
	mu        sync.RWMutex
	perTarget map[string]map[*pooledConnection]*trackedEntry

	subSeq atomic.Int64
	subsMu sync.RWMutex
	subs   map[string]map[int64]*subscriber

	// onTargetStateChange is invoked synchronously on the watcher goroutine
	// before subscriber fan-out. Used by the pool's health helper to push
	// status updates to the health reporter. Set once
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
		subs:         make(map[string]map[int64]*subscriber),
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
			backfill = coreslices.AppendIf(backfill, entry.cancel == nil, pending{target, pc, entry})
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

// shutdown cancels every watcher and waits for them to exit, then stops
// subscriber delivery without waiting for callbacks in progress. Safe to call
// multiple times and from a subscriber callback.
func (t *stateTracker) shutdown() {
	t.mu.Lock()
	t.closed = true
	t.parentCancel()
	t.mu.Unlock()
	t.wg.Wait()

	t.subsMu.Lock()
	for target, subs := range t.subs {
		for _, sub := range subs {
			sub.close()
		}
		delete(t.subs, target)
	}
	t.subsMu.Unlock()
}

// spawnWatcher creates the per-conn context and goroutine. Safe under
// concurrent attach/enable: only the first caller for a given entry wins,
// later callers cancel their own context and return.
func (t *stateTracker) spawnWatcher(target string, pc *pooledConnection, entry *trackedEntry) {
	ctx, cancel := context.WithCancel(t.parentCtx)
	t.mu.Lock()
	targetMap, ok := t.perTarget[target]
	if t.closed || !ok || targetMap[pc] != entry || entry.cancel != nil {
		t.mu.Unlock()
		cancel()
		return
	}
	entry.cancel = cancel
	// Go registers the watcher before the lock is released, so Close cannot
	// miss it between unlock and start.
	t.wg.Go(func() { t.watch(ctx, target, pc, entry) })
	t.mu.Unlock()
}

func (t *stateTracker) watch(ctx context.Context, target string, pc *pooledConnection, entry *trackedEntry) {
	state := pc.conn.GetState()
	t.recordAndFanOut(target, entry, state)
	for pc.conn.WaitForStateChange(ctx, state) {
		state = pc.conn.GetState()
		t.recordAndFanOut(target, entry, state)
	}
}

// recordAndFanOut updates the entry's last-seen state, invokes the
// pool-level callback, and queues the state for the subscribers of this target.
func (t *stateTracker) recordAndFanOut(target string, entry *trackedEntry, state connectivity.State) {
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
	for _, sub := range targetSubs {
		sub.enqueue(state)
	}
	t.subsMu.RUnlock()
}

// subscribe registers cb for the given target and implicitly enables the
// tracker. The returned unsubscribe is idempotent.
func (t *stateTracker) subscribe(target string, cb stateChangeFn) func() {
	if cb == nil {
		return func() {}
	}
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return func() {}
	}
	id := t.subSeq.Add(1)
	sub := newSubscriber(cb)
	t.subsMu.Lock()
	targetSubs, ok := t.subs[target]
	if !ok {
		targetSubs = make(map[int64]*subscriber)
		t.subs[target] = targetSubs
	}
	targetSubs[id] = sub
	t.subsMu.Unlock()

	t.enable()

	return func() {
		sub.close()
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
// all conns currently tracked for target: the watcher's last observation, or
// the live state of a connection without a watcher. Returns
// [connectivity.Idle] when the target is unknown.
func (t *stateTracker) stateForTarget(target string) connectivity.State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	targetMap, ok := t.perTarget[target]
	if !ok || len(targetMap) == 0 {
		return connectivity.Idle
	}
	best := connectivity.Shutdown
	for pc, entry := range targetMap {
		state := entry.state
		if entry.cancel == nil && pc != nil && pc.conn != nil {
			// No watcher keeps entry.state current, so ask the connection.
			state = pc.conn.GetState()
		}
		if connStateRank(state) > connStateRank(best) {
			best = state
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
