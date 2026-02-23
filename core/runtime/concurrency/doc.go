// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package concurrency provides utilities for managing concurrent execution.
// It includes functions for calculating optimal concurrency limits based on
// runtime conditions (CPU, memory, load) and a concurrent batch processor
// for parallelizing work across collections.
//
// Concurrency Calculation:
//
// The package offers several strategies for determining optimal concurrency:
//   - Environment-based: predefined limits for common scenarios (CPU-bound, IO-bound, etc.)
//   - Memory-aware: adjusts limits based on available system memory
//   - Load-aware: adjusts limits based on system load averages
//   - Adaptive: combines multiple factors for complex resource management
//
// Batch Processing:
//
// The batch processor allows concurrent execution of functions over slices with
// configurable concurrency limits and error handling strategies. It supports
// both fixed limits and dynamic calculation via ConcurrencyLimitFunc.
//
// Example Batch Usage:
//
//	items := []string{"a", "b", "c"}
//
//	// Example with adaptive concurrency based on memory
//	config := concurrency.BatchConfig[string]{
//	    LimitFunc:   concurrency.MemoryAwareConcurrency(100, 500, 1000),
//	    StopOnError: true,
//	    OnSuccess: func(item string) {
//	        fmt.Printf("Processed: %s\n", item)
//	    },
//	    OnError: func(item string, err error) {
//	        fmt.Printf("Failed: %s, error: %v\n", item, err)
//	    },
//	}
//
//	// Simple parallel processing
//	err := concurrency.Process(ctx, items, func(ctx context.Context, item string) error {
//	    return processItem(ctx, item)
//	}, config)
//
//	// Parallel transformation and collection
//	results, err := concurrency.ProcessCollect(ctx, items, func(ctx context.Context, item string) (int, error) {
//	    return len(item), nil
//	}, config)
package concurrency
