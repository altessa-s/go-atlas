// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"os"
	"time"
)

// Default configuration constants for [Signal] and its handler execution.
const (
	// DefaultSignalChannelBuffer is the default buffer size for the OS signal
	// channel, ensuring at least one signal can be queued without blocking.
	DefaultSignalChannelBuffer = 1

	// DefaultStopChannelBuffer is the default buffer size for the internal
	// stop channel used during [Signal.Shutdown].
	DefaultStopChannelBuffer = 1

	// DefaultWorkerPoolSize is the default number of concurrent goroutines
	// allowed to execute signal handlers simultaneously.
	DefaultWorkerPoolSize = 10

	// DefaultShutdownTimeout is the maximum time [Signal.Shutdown] waits for
	// in-flight handlers to complete before returning a timeout error.
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultHandlerTimeout is the maximum time an individual [ContextHandler]
	// is allowed to run before being reported as a [*TimeoutError].
	DefaultHandlerTimeout = 5 * time.Second

	// LargeHandlerCountThreshold is the handler count above which the priority
	// queue optimization is used instead of slice sorting.
	LargeHandlerCountThreshold = 50

	// SmallHandlerCountThreshold is the handler count at or below which a
	// simple insertion sort is used (faster than counting sort for small N).
	SmallHandlerCountThreshold = 10

	// ResponsiveTimeoutDuration is the grace period given to parallel handlers
	// to finish after a global shutdown is requested.
	ResponsiveTimeoutDuration = 50 * time.Millisecond
)

// ExecutionMode controls whether handlers for a single signal are run
// sequentially or in parallel. Set it via [WithExecutionMode].
type ExecutionMode int

const (
	// SequentialMode executes handlers one after another in priority order.
	// Higher priority handlers must complete before lower priority handlers start.
	// This is the default mode that preserves backward compatibility and ensures
	// predictable execution order.
	SequentialMode ExecutionMode = iota

	// ParallelMode executes all handlers concurrently in separate goroutines.
	// This provides maximum performance and responsiveness but handlers may
	// complete in any order regardless of priority. Priority still determines
	// startup order but not completion order.
	ParallelMode
)

// options holds Signal configuration.
type options struct {
	signals             []os.Signal
	workerPoolSize      int `optgen:"default=DefaultWorkerPoolSize"`
	errorHandler        ErrorHandler
	shutdownTimeout     time.Duration `optgen:"default=DefaultShutdownTimeout"`
	handlerTimeout      time.Duration `optgen:"default=DefaultHandlerTimeout"`
	signalChannelBuffer int           `optgen:"default=DefaultSignalChannelBuffer"`
	executionMode       ExecutionMode `optgen:"default=SequentialMode"`
}
