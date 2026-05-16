// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package result

// Result holds either a value of type T or an error.
//
// Result is intended for places where Go's idiomatic (T, error) tuple
// is awkward to express: elements of a channel, slice, or map where
// each item independently succeeds or fails. It is NOT a general
// replacement for (T, error) returns; functions should keep returning
// (T, error) and callers should keep using `if err != nil`.
//
// The zero value is a valid Ok of the zero value of T (Get returns
// (zero, nil), IsOk reports true). To represent a failure pass a
// non-nil error to Err or Of; passing a nil error to Err produces a
// Result indistinguishable from Ok of the zero value.
//
// For panic-on-error semantics combine Get with the existing
// panics.MustResult helper:
//
//	v := panics.MustResult(r.Get())
//
// Result deliberately exposes no map/and-then combinators: in Go
// they read as nested closures rather than terse pipelines, so plain
// `if err != nil` stays shorter and clearer. Convert back to
// (T, error) with Get and use ordinary control flow instead.
type Result[T any] struct {
	value T
	err   error
}

// Ok returns a Result containing v with no error.
func Ok[T any](v T) Result[T] {
	return Result[T]{value: v}
}

// Err returns a Result carrying err and the zero value of T. If err
// is nil the resulting Result is indistinguishable from Ok of the
// zero value; callers that mean "failure" must pass a non-nil error.
func Err[T any](err error) Result[T] {
	return Result[T]{err: err}
}

// Of converts an idiomatic (T, error) pair into a Result. It is the
// canonical bridge from existing call sites:
//
//	results <- result.Of(os.ReadFile(path))
func Of[T any](v T, err error) Result[T] {
	return Result[T]{value: v, err: err}
}

// Get returns the underlying (value, error) pair. It is the primary
// accessor and the bridge back to idiomatic Go control flow.
func (r Result[T]) Get() (T, error) {
	return r.value, r.err
}

// Value returns the contained value as-is. When the Result carries
// an error the zero value of T is returned. Use Get when the caller
// must distinguish a real Ok from a default-on-error.
func (r Result[T]) Value() T {
	return r.value
}

// Err returns the contained error, or nil if the Result is Ok.
func (r Result[T]) Err() error {
	return r.err
}

// IsOk reports whether the Result holds a value (Err returns nil).
func (r Result[T]) IsOk() bool {
	return r.err == nil
}

// IsErr reports whether the Result carries an error.
func (r Result[T]) IsErr() bool {
	return r.err != nil
}

// OrDefault returns the contained value if Ok, otherwise def.
func (r Result[T]) OrDefault(def T) T {
	if r.err != nil {
		return def
	}
	return r.value
}

// OrElse returns the contained value if Ok, otherwise the result of
// calling fn(err). fn is invoked only on the error path; it must not
// be nil when the Result may carry an error.
func (r Result[T]) OrElse(fn func(error) T) T {
	if r.err != nil {
		return fn(r.err)
	}
	return r.value
}
