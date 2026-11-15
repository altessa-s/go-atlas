// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"sync"
	"sync/atomic"
)

// inflightLoads tracks the fallback loads currently running, keyed by
// effective key, so that a Delete overlapping a load keeps that load from
// writing its pre-delete result back. An entry lives only while at least one
// load of its key is running, which bounds the map by the number of
// concurrent loads.
type inflightLoads struct {
	// active counts running loads; Delete reads it to skip the lock when no
	// load is in flight.
	active atomic.Int64

	mu    sync.Mutex
	loads map[string]*loadGuard
}

// loadGuard is the state shared by the loads of one key and the deletes that
// overlap them.
type loadGuard struct {
	// mu is held across the invalidation check and the write of a load, and
	// while a delete marks the guard, so a delete either sees the write
	// finished (and removes it) or stops it.
	mu          sync.Mutex
	invalidated bool

	refs int // guarded by inflightLoads.mu
}

// begin registers a load of ek. It must be called before the fallback reads
// the source of truth.
func (l *inflightLoads) begin(ek string) *loadGuard {
	l.mu.Lock()
	defer l.mu.Unlock()

	g := l.loads[ek]
	if g == nil {
		if l.loads == nil {
			l.loads = make(map[string]*loadGuard)
		}
		g = &loadGuard{}
		l.loads[ek] = g
	}
	g.refs++
	l.active.Add(1)
	return g
}

// end unregisters a load started by begin.
func (l *inflightLoads) end(ek string, g *loadGuard) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.active.Add(-1)
	g.refs--
	if g.refs == 0 {
		delete(l.loads, ek)
	}
}

// invalidate marks the running loads of eks so they do not store their
// result. It must be called before the keys are deleted from the storage.
func (l *inflightLoads) invalidate(eks ...string) {
	if l.active.Load() == 0 {
		return
	}
	for _, ek := range eks {
		l.mu.Lock()
		g := l.loads[ek]
		l.mu.Unlock()
		if g == nil {
			continue
		}
		g.mu.Lock()
		g.invalidated = true
		g.mu.Unlock()
	}
}
