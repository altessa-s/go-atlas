// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/data/leadelect"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

const (
	// DefaultTickInterval is the default interval between scheduler loop iterations.
	// See [WithTickInterval].
	DefaultTickInterval = 1 * time.Second
	// DefaultHistoryRetention is the default duration for which completed task
	// execution history is retained before being purged. See [WithHistoryRetention].
	DefaultHistoryRetention = 7 * 24 * time.Hour
	// DefaultMaxConcurrentTasks is the default upper bound on concurrent task
	// executions. Zero means unlimited. See [WithMaxConcurrentTasks].
	DefaultMaxConcurrentTasks = 0
	// DefaultReservedHighPrioritySlots is the default number of concurrency slots
	// reserved exclusively for [TaskPriorityHigh] tasks. Normal and Low priority
	// tasks cannot consume these slots. See [WithReservedHighPrioritySlots].
	DefaultReservedHighPrioritySlots = 2
	// DefaultStaleTaskTimeout is the default duration after which a task in
	// [TaskStatusRunning] is considered stale and reset to [TaskStatusActive]
	// during periodic recovery. See [WithStaleTaskTimeout].
	DefaultStaleTaskTimeout = 30 * time.Minute
	// maxStaleRecoveryInterval caps the stale recovery ticker interval.
	maxStaleRecoveryInterval = 5 * time.Minute
)

type options struct {
	tickInterval              time.Duration `optgen:"default=DefaultTickInterval"`
	historyRetention          time.Duration `optgen:"default=DefaultHistoryRetention"`
	staleTaskTimeout          time.Duration `optgen:"default=DefaultStaleTaskTimeout"`
	maxConcurrentTasks        int           `optgen:"default=DefaultMaxConcurrentTasks"`
	reservedHighPrioritySlots int           `optgen:"default=DefaultReservedHighPrioritySlots"`
	logger                    *slog.Logger
	leaderElector             leadelect.LeaderElector          `optgen:"notnil" optval:"nil"`
	concurrencyLimitFunc      concurrency.ConcurrencyLimitFunc `opt:"-"`
}

// WithConcurrencyLimitFunc sets a dynamic concurrency limit function that is
// evaluated on each scheduler tick. When set, this takes precedence over
// [WithMaxConcurrentTasks], switching the scheduler to dynamic concurrency mode
// where no blocking semaphores are allocated. The function should return the
// maximum number of non-critical tasks that may execute concurrently; zero or
// negative means unlimited.
//
// Use with functions from the concurrency package:
//
//	scheduler.WithConcurrencyLimitFunc(
//	    concurrency.AdaptiveConcurrency(concurrency.AdaptiveConcurrencyConfig{
//	        MemoryLowThresholdMB:    256,
//	        MemoryMediumThresholdMB: 512,
//	    }),
//	)
func WithConcurrencyLimitFunc(fn concurrency.ConcurrencyLimitFunc) Option {
	return func(o *options) {
		o.concurrencyLimitFunc = fn
	}
}

// WithEnvironment sets the concurrency limit based on a predefined environment
// profile. This is a convenience wrapper around [WithConcurrencyLimitFunc] that
// creates a fixed-value function from the profile. The limit is computed once
// at option creation time and does not change at runtime.
//
// Available environments:
//   - EnvironmentMemoryConstrained: 1 worker
//   - EnvironmentCPUBound: runtime.NumCPU()
//   - EnvironmentIOBound: runtime.NumCPU() * 2
//   - EnvironmentHighThroughput: runtime.NumCPU() * 4
//   - EnvironmentRateLimited: 3 workers
func WithEnvironment(env concurrency.Environment) Option {
	limit := concurrency.ConcurrencyForEnvironment(env)
	return func(o *options) {
		o.concurrencyLimitFunc = func() int { return limit }
	}
}
