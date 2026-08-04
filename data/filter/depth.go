// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import coreerrs "github.com/altessa-s/go-atlas/core/errors"

// DepthGuard bounds how deeply a [Visitor] recurses into an expression.
//
// A filter arrives from outside the process, so its nesting is an input
// like any other: unbounded, a few kilobytes of parentheses become a
// stack overflow. Every translator in this repository guards its
// recursive Visit methods with one, which is why the guard lives here
// rather than being written out five times.
//
// A guard is per-call state and is therefore not safe for concurrent
// use — neither is any translator that holds one. The zero value has a
// limit of zero and refuses everything; construct with [NewDepthGuard].
type DepthGuard struct {
	depth int
	max   int
}

// NewDepthGuard returns a guard that admits limit levels of nesting.
// Callers take the limit from [TranslatorContext.MaxDepth], which
// carries the [WithMaxDepth] option or [DefaultMaxDepth].
func NewDepthGuard(limit int) DepthGuard {
	return DepthGuard{max: limit}
}

// Enter records one level of nesting, or reports [ErrMaxDepthExceeded]
// when the limit is already reached. A successful Enter must be paired
// with a deferred [DepthGuard.Leave]:
//
//	if err := t.depth.Enter(); err != nil {
//	    return nil, err
//	}
//	defer t.depth.Leave()
//
// A failed Enter records nothing, so it must not be paired with a Leave.
func (g *DepthGuard) Enter() error {
	if g.depth >= g.max {
		return coreerrs.Wrapf(ErrMaxDepthExceeded, "depth %d exceeds maximum %d", g.depth, g.max)
	}
	g.depth++
	return nil
}

// Leave undoes one [DepthGuard.Enter].
func (g *DepthGuard) Leave() {
	g.depth--
}

// Reset returns the guard to zero depth. A translator that is reused
// across calls does this at the start of each one, so a walk that
// aborted mid-expression does not leave the next one short of budget.
func (g *DepthGuard) Reset() {
	g.depth = 0
}
