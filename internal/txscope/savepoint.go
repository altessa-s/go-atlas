// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package txscope

import (
	"context"
	"sync/atomic"
)

type scopeKey struct{ Key }

// Savepoint identifies a nested rollback boundary. A nil pointer identifies
// the transaction body outside any savepoint.
type Savepoint struct {
	parent *Savepoint
	state  atomic.Uint32
}

const (
	open uint32 = iota
	sealed
	released
	discarded
)

// Current returns the innermost savepoint in ctx.
func Current(ctx context.Context) *Savepoint {
	scope, _ := ctx.Value(scopeKey{}).(*Savepoint)
	return scope
}

// Begin opens a child of Current(ctx). The adapter calls finish exactly once,
// including on panic, passing true only after the database releases the child.
func Begin(ctx context.Context) (context.Context, func(bool)) {
	s := &Savepoint{parent: Current(ctx)}
	return context.WithValue(ctx, scopeKey{}, s), func(success bool) {
		if success {
			s.state.Store(released)
		} else {
			s.state.Store(discarded)
		}
	}
}

// Seal ends execution within the savepoint, before its database outcome is
// known. The adapter must still finish the scope after release or rollback.
func (s *Savepoint) Seal() {
	s.state.CompareAndSwap(open, sealed)
}

// OpenWithin reports whether every scope entered since ancestor is still open.
func (s *Savepoint) OpenWithin(ancestor *Savepoint) bool {
	return s.within(ancestor, open)
}

// ReleasedWithin reports whether the database released every scope entered
// since ancestor. Releasing a child alone cannot survive its parent's rollback.
func (s *Savepoint) ReleasedWithin(ancestor *Savepoint) bool {
	return s.within(ancestor, released)
}

func (s *Savepoint) within(ancestor *Savepoint, state uint32) bool {
	for s != ancestor {
		if s == nil || s.state.Load() != state {
			return false
		}
		s = s.parent
	}
	return true
}
