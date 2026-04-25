// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import "iter"

// Noop implements [Filter] as a no-op: [Noop.ShouldFilter] always returns
// false, and its iterators yield nothing. Use it as a safe default when
// endpoint filtering is disabled or when [New] fails.
type Noop struct{}

// NewNoop creates a new Noop checker instance.
//
// Example:
//
//	checker := endpointfilter.NewNoop()
func NewNoop() *Noop { return &Noop{} }

// ShouldFilter always returns false.
func (nch *Noop) ShouldFilter(_ string) bool { return false }

// Paths returns an empty iterator.
func (nch *Noop) Paths() iter.Seq[string] {
	return func(yield func(string) bool) {}
}

// Methods returns an empty iterator.
func (nch *Noop) Methods() iter.Seq[string] {
	return func(yield func(string) bool) {}
}
