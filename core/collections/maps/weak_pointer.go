// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

import "weak"

// WeakRef is a thin wrapper around [weak.Pointer] that provides a convenience API
// for holding a weak reference to a heap-allocated object of type T. The referenced
// object may be garbage collected at any time if no strong references remain.
//
// A zero-value WeakRef behaves as if the referenced object has already been collected
// (i.e., [WeakRef.Value] returns nil and [WeakRef.IsAlive] returns false).
type WeakRef[T any] struct {
	inner weak.Pointer[T]
}

// MakeWeakRef creates a new [WeakRef] pointing to obj. If obj is nil, the returned
// WeakRef is equivalent to its zero value (already "dead"). The reference does not
// prevent obj from being garbage collected.
func MakeWeakRef[T any](obj *T) WeakRef[T] {
	if obj == nil {
		return WeakRef[T]{}
	}
	return WeakRef[T]{
		inner: weak.Make(obj),
	}
}

// Value returns a pointer to the referenced object, or nil if the object has been
// garbage collected. A non-nil return value keeps the object alive for at least as
// long as the returned pointer is reachable.
func (r WeakRef[T]) Value() *T {
	return r.inner.Value()
}

// IsAlive reports whether the referenced object has not yet been garbage collected.
// Note that the result may become stale immediately after the call returns; use
// [WeakRef.Value] and check for nil when you need the actual object.
func (r WeakRef[T]) IsAlive() bool {
	return r.inner.Value() != nil
}
