// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ptr

import (
	"github.com/altessa-s/go-atlas/core/types/constraints"
)

// Wrap returns a pointer to a copy of the provided value. This is primarily useful
// for creating pointers to literals, constants, or inline expressions where the
// address-of operator (&) cannot be applied directly. The type parameter T must
// satisfy [constraints.Primitive]: the constraint is deliberately narrower than
// any because these helpers target scalar values (numbers, strings, booleans);
// for arbitrary types take the address with &v directly. See also [WrapNonZero]
// to skip zero values and [Unwrap] for safe dereferencing.
//
// Example:
//
//	strPtr := ptr.Wrap("hello")
func Wrap[T constraints.Primitive](s T) *T {
	return &s
}

// WrapNonZero returns a pointer to a copy of the provided value only if it is not
// the zero value for its type. Returns nil for zero values (0 for numbers, false
// for booleans, "" for strings). This is useful for optional fields in structs
// where nil indicates "not set" and the zero value is a valid but absent state.
// See also [Wrap] which always creates a pointer regardless of the value.
//
// Example:
//
//	ptr.WrapNonZero(42)  // *int pointing to 42
//	ptr.WrapNonZero(0)   // nil
func WrapNonZero[T constraints.Primitive](s T) *T {
	var zero T
	if s == zero {
		return nil
	}

	return &s
}

// Unwrap safely dereferences a pointer, returning its pointed-to value. If the
// pointer is nil, it returns the first element of dv as the default, or the zero
// value of T if no default is provided. Only the first element of dv is used;
// additional values are ignored. This avoids nil pointer panics when reading
// optional fields. See also [Wrap] and [WrapNonZero].
//
// Example:
//
//	ptr.Unwrap(ptr.Wrap(42))     // 42
//	ptr.Unwrap[int](nil)         // 0
//	ptr.Unwrap[int](nil, 99)     // 99
func Unwrap[T constraints.Primitive](p *T, dv ...T) T {
	if p != nil {
		return *p
	}

	var t T
	if len(dv) > 0 {
		t = dv[0]
	}
	return t
}
