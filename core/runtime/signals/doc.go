// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package signals provides OS signal handling with priority-based execution and rate limiting.
// Supports graceful shutdown, context-aware handlers, and concurrent processing.
//
// Fully thread-safe. All operations (registration, start/stop, signal processing)
// can be called concurrently without external synchronization.
//
// # Configuration
//
// Options passed to New():
//   - WithSignals(...os.Signal): Which signals to listen for.
//   - WithShutdownTimeout(duration): Graceful shutdown timeout (default: 30s).
//   - WithHandlerTimeout(duration): Individual handler timeout (default: 5s).
//   - WithExecutionMode(mode): Sequential or Parallel (default: Sequential).
//
// # Usage
//
//	handler := signals.New(
//	    signals.WithSignals(syscall.SIGTERM, syscall.SIGINT),
//	    signals.WithShutdownTimeout(10*time.Second),
//	)
//
//	handler.AddHandler(func(ctx context.Context, sig os.Signal) error {
//	    fmt.Println("Shutting down...")
//	    return cleanup()
//	}, syscall.SIGTERM, syscall.SIGINT)
//
//	handler.Start()
//	handler.Wait() // Block until signal received
//
// # Performance
//
//   - Worker pool limits concurrent handler execution to prevent resource exhaustion.
//   - Parallel mode executes independent handlers concurrently (up to 3x speedup).
//   - Rate limiting uses token bucket algorithm with atomic operations.
package signals
