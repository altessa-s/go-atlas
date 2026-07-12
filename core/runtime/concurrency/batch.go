// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

import (
	"context"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ProcessFunc is the callback signature for [Process]. It receives a
// context (which may be canceled on error when [WithStopOnError] is
// set) and a single item to process. Returning a non-nil error signals
// a failure for that item.
type ProcessFunc[T any] func(ctx context.Context, item T) error

// TransformFunc is the callback signature for [ProcessCollect]. It
// receives a context and a single item, returning a transformed result
// or an error.
type TransformFunc[T, R any] func(ctx context.Context, item T) (R, error)

// Process applies fn to every element of items using bounded concurrency
// controlled by opts. When [WithStopOnError] is set, the first error
// cancels remaining work and is returned; otherwise only the first
// error encountered is returned while all items are still processed.
// If items is empty, nil is returned immediately. When the effective
// concurrency is 1 (or there is only one item), processing is sequential
// in the caller's goroutine.
//
// Example:
//
//	err := Process(ctx, items, func(ctx context.Context, item string) error {
//	    return processItem(item)
//	}, WithConcurrency[string](4), WithStopOnError[string]())
func Process[T any](
	ctx context.Context,
	items []T,
	fn ProcessFunc[T],
	opts ...Option[T],
) error {
	if len(items) == 0 {
		return nil
	}
	cfg := newOptions(opts...)
	return processWithOptions(ctx, items, fn, cfg)
}

// processWithOptions is the shared implementation of [Process] that
// accepts an already-materialized options value. It is used by
// [ProcessCollect] to avoid re-applying option functions.
func processWithOptions[T any](
	ctx context.Context,
	items []T,
	fn ProcessFunc[T],
	cfg *options[T],
) error {
	concurrency := getConcurrency(cfg)
	if concurrency == 1 || len(items) == 1 {
		return processSequential(ctx, items, fn, cfg)
	}

	// Internal context to cancel all workers if stopOnError is true
	gCtx := ctx
	var cancel context.CancelFunc
	if cfg.stopOnError {
		gCtx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	var once sync.Once
	var firstErr error

	// Mutex for OnSuccess/OnError callbacks
	var cbMx sync.Mutex

	handle := func(item T) {
		if err := fn(gCtx, item); err != nil {
			once.Do(func() {
				firstErr = err
				if cfg.stopOnError && cancel != nil {
					cancel() // Stop other workers
				}
			})

			if cfg.onError != nil {
				cbMx.Lock()
				cfg.onError(item, err)
				cbMx.Unlock()
			}
		} else if cfg.onSuccess != nil {
			cbMx.Lock()
			cfg.onSuccess(item)
			cbMx.Unlock()
		}
	}

	// A fixed pool of workers consumes item indices from an unbuffered
	// channel, so a batch spawns min(concurrency, len(items)) goroutines
	// instead of one per item. The unbuffered channel keeps the dispatch
	// gating identical to the former semaphore: an index handed over is an
	// item being processed, and nothing is queued past a cancellation.
	var wg sync.WaitGroup
	indexes := make(chan int)
	for range min(concurrency, len(items)) {
		wg.Go(func() {
			for idx := range indexes {
				handle(items[idx])
			}
		})
	}

feed:
	for i := range items {
		// Check if context is already canceled
		if gCtx.Err() != nil {
			break
		}

		select {
		case indexes <- i:
		case <-gCtx.Done():
			break feed
		}
	}
	close(indexes)

	wg.Wait()
	return firstErr
}

// ProcessCollect applies fn to every element of items concurrently
// (like [Process]) and collects the transformed results into a slice
// that preserves the original input order. On error with
// [WithStopOnError] set, partial results collected so far are returned
// alongside the error.
//
// Internally, ProcessCollect processes item indices rather than wrapped
// structs, writes results directly into a pre-allocated array by
// position (no mutex needed since each goroutine writes to a unique
// index), and skips the sort step entirely.
func ProcessCollect[T, R any](
	ctx context.Context,
	items []T,
	fn TransformFunc[T, R],
	opts ...Option[T],
) ([]R, error) {
	n := len(items)
	if n == 0 {
		return []R{}, nil
	}

	cfg := newOptions(opts...)

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

	// Translate options[T] → options[int] so we can reuse processWithOptions.
	idxCfg := &options[int]{
		concurrency: cfg.concurrency,
		limitFunc:   cfg.limitFunc,
		stopOnError: cfg.stopOnError,
	}
	if cfg.onSuccess != nil {
		idxCfg.onSuccess = func(idx int) { cfg.onSuccess(items[idx]) }
	}
	if cfg.onError != nil {
		idxCfg.onError = func(idx int, err error) { cfg.onError(items[idx], err) }
	}

	err := processWithOptions(ctx, indices, func(gCtx context.Context, idx int) error {
		res, fnErr := fn(gCtx, items[idx])
		if fnErr != nil {
			return fnErr
		}
		results[idx] = res
		completed[idx] = true
		return nil
	}, idxCfg)

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
		// All completed despite error (stopOnError=false, error from a single item).
		if cfg.stopOnError {
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

	if cfg.stopOnError {
		return compacted, coreerrs.Wrap(err, "batch processing failed")
	}
	return compacted, err
}

// processSequential processes items one by one in the current goroutine.
// It mirrors the parallel path's stopOnError semantics: when unset, all
// items are still processed and the first error encountered is returned.
func processSequential[T any](
	ctx context.Context,
	items []T,
	fn ProcessFunc[T],
	cfg *options[T],
) error {
	var firstErr error
	for _, item := range items {
		if err := fn(ctx, item); err != nil {
			if cfg.onError != nil {
				cfg.onError(item, err)
			}
			if cfg.stopOnError {
				return err
			}
			if firstErr == nil {
				firstErr = err
			}
		} else if cfg.onSuccess != nil {
			cfg.onSuccess(item)
		}
		if ctx.Err() != nil {
			if firstErr != nil {
				return firstErr
			}
			return ctx.Err()
		}
	}
	return firstErr
}

func getConcurrency[T any](cfg *options[T]) int {
	// 1. If explicit function is provided, use it.
	if cfg.limitFunc != nil {
		if limit := cfg.limitFunc(); limit > 0 {
			return limit
		}
	}

	// 2. If explicit fixed number is provided, use it.
	if cfg.concurrency > 0 {
		return cfg.concurrency
	}

	// 3. Fallback to default adaptive function (IO-bound).
	return DefaultLimitFunc()
}
