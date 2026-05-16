// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional

// Optional holds either a value of type T or nothing.
//
// Optional makes the "value may be absent" intent explicit at the
// type level. It is intended for places where Go's idiomatic
// (T, bool) pair or *T is awkward — struct fields, channel
// elements, slice values, map values — not as a general replacement
// for `value, ok := m[k]` or `if p != nil`.
//
// Internally Optional carries (value, present) by value: it never
// boxes the contained value, so Some(v) does not allocate and the
// type is comparable when T is comparable. The zero value of
// Optional[T] is a valid None.
//
// Optional deliberately exposes no map/and-then combinators: in Go
// they read as nested closures rather than terse pipelines, so plain
// `if ok` stays shorter and clearer. Convert back to (T, bool) with
// Get and use ordinary control flow instead.
type Optional[T any] struct {
	value   T
	present bool
}

// Some returns an Optional carrying v.
func Some[T any](v T) Optional[T] {
	return Optional[T]{value: v, present: true}
}

// None returns an empty Optional. The zero value of Optional[T] is
// equivalent to None[T]().
func None[T any]() Optional[T] {
	return Optional[T]{}
}

// Of converts an idiomatic (T, bool) pair into an Optional. It is
// the canonical bridge from existing call sites:
//
//	opt := optional.Of(m[k])
//
// When ok is false v is discarded and the resulting Optional carries
// the zero value of T.
func Of[T any](v T, ok bool) Optional[T] {
	if !ok {
		return Optional[T]{}
	}
	return Optional[T]{value: v, present: true}
}

// Get returns the underlying (value, present) pair. It is the
// primary accessor and the bridge back to idiomatic Go control flow.
func (o Optional[T]) Get() (T, bool) {
	return o.value, o.present
}

// Value returns the contained value as-is. When the Optional is None
// the zero value of T is returned. Use Get when the caller must
// distinguish a real Some(zero) from None.
func (o Optional[T]) Value() T {
	return o.value
}

// IsSome reports whether the Optional carries a value.
func (o Optional[T]) IsSome() bool {
	return o.present
}

// IsNone reports whether the Optional is empty.
func (o Optional[T]) IsNone() bool {
	return !o.present
}

// OrDefault returns the contained value if Some, otherwise def.
func (o Optional[T]) OrDefault(def T) T {
	if !o.present {
		return def
	}
	return o.value
}

// OrElse returns the contained value if Some, otherwise the result
// of calling fn. fn is invoked only on the None path; it must not be
// nil when the Optional may be None.
func (o Optional[T]) OrElse(fn func() T) T {
	if !o.present {
		return fn()
	}
	return o.value
}
