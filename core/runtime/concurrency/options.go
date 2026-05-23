// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// SuccessCallback is invoked by [Process] and [ProcessCollect] after an
// item has been processed without error. The callback runs while
// holding an internal mutex, so it is safe to mutate shared state from
// within it.
type SuccessCallback[T any] func(item T)

// ErrorCallback is invoked by [Process] and [ProcessCollect] after an
// item has failed. The callback runs while holding an internal mutex,
// so it is safe to mutate shared state from within it.
type ErrorCallback[T any] func(item T, err error)

// options holds the tunable policy used by [Process] and
// [ProcessCollect]. Configure it through the generated [Option] values
// (see the With* constructors below).
type options[T any] struct {
	// concurrency sets a fixed upper bound on the number of goroutines
	// that process items simultaneously. It is ignored when limitFunc
	// is set. Zero or negative values fall through to the default
	// adaptive concurrency limit.
	concurrency int //

	// limitFunc, when non-nil, is called to determine the concurrency
	// limit dynamically and takes precedence over concurrency. Use one
	// of the built-in factories such as [MemoryAwareConcurrency] or
	// [AdaptiveConcurrency] to create a suitable function.
	limitFunc ConcurrencyLimitFunc //

	// stopOnError, when true, cancels the internal context after the
	// first error, preventing new items from starting. Items already in
	// flight may still complete. Only the first error is returned.
	stopOnError bool //

	// onSuccess is an optional callback invoked after each item
	// completes without error.
	onSuccess SuccessCallback[T] //

	// onError is an optional callback invoked after each item that
	// fails.
	onError ErrorCallback[T] //
}
