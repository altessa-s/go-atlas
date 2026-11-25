// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"slices"
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
	gate        chan struct{}
	invalidated bool

	refs int // guarded by inflightLoads.mu
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
	for _, guard := range guards {
		guard.invalidated = true
	}
	return err
}
