// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"slices"
	"strconv"
	"sync"
)

// inflightLoads coordinates fallback loads and explicit mutations by effective
// key. An entry lives only while an operation on that key holds a reference.
type inflightLoads struct {
	mu    sync.Mutex
	loads map[string]*loadGuard
}

// loadGuard is the state shared by the loads of one key and explicit mutations that
// overlap them.
type loadGuard struct {
	// gate is held across provider writes so overlapping mutations have a
	// consistent order. A waiting caller can leave on context cancellation.
	gate chan struct{}

	refs  int    // guarded by inflightLoads.mu
	epoch uint64 // guarded by inflightLoads.mu; advanced by every write that invalidates loads
}

// epochOf returns the current epoch of g. A load remembers the epoch it began
// under and does not store its result once the epoch has moved on.
func (l *inflightLoads) epochOf(g *loadGuard) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return g.epoch
}

// advance moves the epoch of every guard on, which invalidates the loads that
// began under the previous one and separates later flights from them.
func (l *inflightLoads) advance(guards ...*loadGuard) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, g := range guards {
		g.epoch++
	}
}

// flightKey returns the singleflight key of a load of ek under g. It carries
// the epoch of g, so a reader starting after an invalidation never joins a
// flight that read the source before it. The caller holds g (see begin) until
// the flight and every waiter on it have returned, so the epoch cannot reset
// while a flight keyed by it can still be joined.
func (l *inflightLoads) flightKey(ek string, g *loadGuard) string {
	return strconv.FormatUint(l.epochOf(g), 10) + ":" + ek
}

func (g *loadGuard) lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case g.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			g.unlock()
			return err
		}
		return nil
	}
}

func (g *loadGuard) unlock() { <-g.gate }

// begin registers an operation on ek. Call it before the fallback reads the
// source of truth or before an explicit mutation reaches storage.
func (l *inflightLoads) begin(ek string) *loadGuard {
	l.mu.Lock()
	defer l.mu.Unlock()

	g := l.loads[ek]
	if g == nil {
		if l.loads == nil {
			l.loads = make(map[string]*loadGuard)
		}
		g = &loadGuard{gate: make(chan struct{}, 1)}
		l.loads[ek] = g
	}
	g.refs++
	return g
}

// end unregisters a load started by begin.
func (l *inflightLoads) end(ek string, g *loadGuard) {
	l.mu.Lock()
	defer l.mu.Unlock()

	g.refs--
	if g.refs == 0 {
		delete(l.loads, ek)
	}
}

// invalidate runs a delete while holding the affected keys and prevents
// overlapping loads from repopulating them. Consistent lock order lets
// concurrent batches overlap in any caller-provided order.
func (l *inflightLoads) invalidate(ctx context.Context, keys []string, remove func() error) error {
	if len(keys) > 1 {
		keys = slices.Clone(keys)
		slices.Sort(keys)
		keys = slices.Compact(keys)
	}
	guards := make([]*loadGuard, 0, len(keys))
	defer func() {
		for i, guard := range slices.Backward(guards) {
			guard.unlock()
			l.end(keys[i], guard)
		}
	}()
	for _, key := range keys {
		guard := l.begin(key)
		if err := guard.lock(ctx); err != nil {
			l.end(key, guard)
			return err
		}
		guards = append(guards, guard)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := remove()
	// A failed delete may have reached storage or removed part of a batch.
	l.advance(guards...)
	return err
}
