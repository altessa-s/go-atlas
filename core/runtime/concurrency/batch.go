// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

import (
	"context"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ProcessFunc is the callback signature for [Process]. It receives a context
// (which may be canceled on error when [BatchConfig.StopOnError] is set) and
// a single item to process. Returning a non-nil error signals a failure for that item.
type ProcessFunc[T any] func(ctx context.Context, item T) error

// TransformFunc is the callback signature for [ProcessCollect]. It receives a
// context and a single item, returning a transformed result or an error.
type TransformFunc[T, R any] func(ctx context.Context, item T) (R, error)

// BatchConfig configures the concurrency and error-handling behavior of
// [Process] and [ProcessCollect].
type BatchConfig[T any] struct {
	// Concurrency sets a fixed upper bound on the number of goroutines
	// that process items simultaneously. It is ignored when LimitFunc is
	// provided. Zero or negative values fall through to [DefaultLimitFunc].
	Concurrency int

	// LimitFunc, when non-nil, is called to determine the concurrency limit
	// dynamically and takes precedence over the Concurrency field. Use one of
	// the built-in factories such as [MemoryAwareConcurrency] or
	// [AdaptiveConcurrency] to create a suitable function.
	LimitFunc ConcurrencyLimitFunc

	// StopOnError, when true, cancels the internal context after the first
	// error, preventing new items from starting. Items already in flight may
	// still complete. Only the first error is returned by [Process].
	StopOnError bool

	// OnSuccess is an optional callback invoked after each item completes
	// without error. It is called while holding an internal mutex, so it is
	// safe to mutate shared state from within it.
	OnSuccess func(item T)

	// OnError is an optional callback invoked after each item that fails.
	// It is called while holding an internal mutex, so it is safe to mutate
	// shared state from within it.
	OnError func(item T, err error)
}

// Process applies fn to every element of items using bounded concurrency
// controlled by config. When [BatchConfig.StopOnError] is true, the first
// error cancels remaining work and is returned; otherwise only the first
// error encountered is returned while all items are still processed.
// If items is empty, nil is returned immediately. When the effective
// concurrency is 1 (or there is only one item), processing is sequential
// in the caller's goroutine.
//
// Example:
//
//	err := Process(ctx, items, func(ctx context.Context, item string) error {
//	    return processItem(item)
//	}, BatchConfig[string]{Concurrency: 4, StopOnError: true})
func Process[T any](
	ctx context.Context,
	items []T,
	fn ProcessFunc[T],
	config BatchConfig[T],
) error {
	if len(items) == 0 {
		return nil
	}

	concurrency := getConcurrency(config)
	if concurrency == 1 || len(items) == 1 {
		return processSequential(ctx, items, fn, config.OnSuccess, config.OnError)
	}

	// Internal context to cancel all workers if StopOnError is true
	gCtx := ctx
	var cancel context.CancelFunc
	if config.StopOnError {
		gCtx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error

	// Semaphore to limit concurrency
	sem := make(chan struct{}, concurrency)

	// Mutex for OnSuccess/OnError callbacks
	var cbMx sync.Mutex

loop:
	for _, item := range items {
		// Check if context is already canceled
		if gCtx.Err() != nil {
			break
		}

		select {
		case sem <- struct{}{}:
			wg.Go(func() {
				defer func() { <-sem }()

				if err := fn(gCtx, item); err != nil {
					once.Do(func() {
						firstErr = err
						if config.StopOnError && cancel != nil {
							cancel() // Stop other workers
						}
					})

					if config.OnError != nil {
						cbMx.Lock()
						config.OnError(item, err)
						cbMx.Unlock()
					}
				} else if config.OnSuccess != nil {
					cbMx.Lock()
					config.OnSuccess(item)
					cbMx.Unlock()
				}
			})
		case <-gCtx.Done():
			break loop
		}
	}

	wg.Wait()
	return firstErr
}

// ProcessCollect applies fn to every element of items concurrently (like
// [Process]) and collects the transformed results into a slice that preserves
// the original input order. On error with [BatchConfig.StopOnError] set,
// partial results collected so far are returned alongside the error.
//
// Internally, ProcessCollect processes item indices rather than wrapped
// structs, writes results directly into a pre-allocated array by position
// (no mutex needed since each goroutine writes to a unique index), and
// skips the sort step entirely.
func ProcessCollect[T, R any](
	ctx context.Context,
	items []T,
	fn TransformFunc[T, R],
	config BatchConfig[T],
) ([]R, error) {
	n := len(items)
	if n == 0 {
		return []R{}, nil
	}

	// Pre-allocate result array indexed by position.
	// Each goroutine writes to a unique index — no mutex needed.
	results := make([]R, n)

	// Track which indices completed successfully for partial-result support.
	// Written by individual goroutines (one writer per index), read after
	// Process returns (wg.Wait provides happens-before guarantee).
	completed := make([]bool, n)

	// Process indices instead of wrapped items.
	// An int (8 bytes) is much cheaper than struct{int, T} for large T.
	indices := make([]int, n)
	for i := range n {
		indices[i] = i
	}

	// Adapt config callbacks to map indices back to original items.
	idxConfig := BatchConfig[int]{
		Concurrency: config.Concurrency,
		LimitFunc:   config.LimitFunc,
		StopOnError: config.StopOnError,
	}
	if config.OnSuccess != nil {
		idxConfig.OnSuccess = func(idx int) { config.OnSuccess(items[idx]) }
	}
	if config.OnError != nil {
		idxConfig.OnError = func(idx int, err error) { config.OnError(items[idx], err) }
	}

	err := Process(ctx, indices, func(gCtx context.Context, idx int) error {
		res, fnErr := fn(gCtx, items[idx])
		if fnErr != nil {
			return fnErr
		}
		results[idx] = res
		completed[idx] = true
		return nil
	}, idxConfig)

	// Fast path: all items completed successfully (common case).
	if err == nil {
		return results, nil
	}

	// Error path: compact results to only include successfully completed items,
	// preserving the original order.
	count := 0
	for i := range n {
		if completed[i] {
			count++
		}
	}

	if count == n {
		// All completed despite error (StopOnError=false, error from a single item).
		if config.StopOnError {
			return results, coreerrs.Wrap(err, "batch processing failed")
		}
		return results, err
	}

	compacted := make([]R, 0, count)
	for i := range n {
		if completed[i] {
			compacted = append(compacted, results[i])
		}
	}

	if config.StopOnError {
		return compacted, coreerrs.Wrap(err, "batch processing failed")
	}
	return compacted, err
}

// processSequential processes items one by one in the current goroutine.
func processSequential[T any](
	ctx context.Context,
	items []T,
	fn ProcessFunc[T],
	onSuccess func(T),
	onError func(T, error),
) error {
	for _, item := range items {
		if err := fn(ctx, item); err != nil {
			if onError != nil {
				onError(item, err)
			}
			return err
		}
		if onSuccess != nil {
			onSuccess(item)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func getConcurrency[T any](config BatchConfig[T]) int {
	// 1. If explicit function is provided, use it.
	if config.LimitFunc != nil {
		if limit := config.LimitFunc(); limit > 0 {
			return limit
		}
	}

	// 2. If explicit fixed number is provided, use it.
	if config.Concurrency > 0 {
		return config.Concurrency
	}

	// 3. Fallback to default adaptive function (IO-bound).
	return DefaultLimitFunc()
}
