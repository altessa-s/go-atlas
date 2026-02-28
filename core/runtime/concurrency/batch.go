// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

import (
	"context"
	"slices"
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
func ProcessCollect[T, R any](
	ctx context.Context,
	items []T,
	fn TransformFunc[T, R],
	config BatchConfig[T],
) ([]R, error) {
	if len(items) == 0 {
		return []R{}, nil
	}

	type indexedResult struct {
		index int
		val   R
	}

	collected := make([]indexedResult, 0, len(items))
	var mx sync.Mutex

	// Wrap items with index to preserve order
	type indexedItem struct {
		index int
		val   T
	}

	wrappedItems := make([]indexedItem, len(items))
	for i, item := range items {
		wrappedItems[i] = indexedItem{index: i, val: item}
	}

	// Adapt config for wrapped items
	wrappedConfig := BatchConfig[indexedItem]{
		Concurrency: config.Concurrency,
		LimitFunc:   config.LimitFunc,
		StopOnError: config.StopOnError,
	}

	if config.OnSuccess != nil {
		wrappedConfig.OnSuccess = func(item indexedItem) {
			config.OnSuccess(item.val)
		}
	}
	if config.OnError != nil {
		wrappedConfig.OnError = func(item indexedItem, err error) {
			config.OnError(item.val, err)
		}
	}

	// Wrap fn to collect results with index
	err := Process(ctx, wrappedItems, func(gCtx context.Context, item indexedItem) error {
		res, err := fn(gCtx, item.val)
		if err != nil {
			return err
		}

		mx.Lock()
		collected = append(collected, indexedResult{index: item.index, val: res})
		mx.Unlock()
		return nil
	}, wrappedConfig)

	// Even on error with StopOnError, we return what we have collected.
	// Existing implementation returned partial results, so we do the same but ordered.
	_ = err // Error is handled by returning partial results below

	// Sort by index to restore original order
	slices.SortFunc(collected, func(a, b indexedResult) int {
		return a.index - b.index
	})

	// Unwrap results
	results := make([]R, len(collected))
	for i, res := range collected {
		results[i] = res.val
	}

	if err != nil && config.StopOnError {
		return results, coreerrs.Wrap(err, "batch processing failed")
	}

	return results, err
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
