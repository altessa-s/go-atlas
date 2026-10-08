// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

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
	// DefaultStaleTaskTimeout is the default lease of a task run. The instance
	// executing a run renews its lease every third of this duration (at least
	// [minRunLease]); a run whose lease has expired — its instance crashed or
	// lost its storage — is reset to [TaskStatusActive] by stale recovery on any
	// instance. See [WithStaleTaskTimeout].
	DefaultStaleTaskTimeout = 30 * time.Minute
	// DefaultCleanupInterval is the default interval between periodic history
	// cleanup runs. See [WithCleanupInterval].
	DefaultCleanupInterval = 1 * time.Hour
	// DefaultStorageTimeout is the default per-operation deadline applied to
	// every [Storage] call the scheduler makes on its own behalf.
	// See [WithStorageTimeout].
	DefaultStorageTimeout = 10 * time.Second
	// DefaultRunOnStartGrace is the default window within which a recent run
	// suppresses a RunOnStart run. See [WithRunOnStartGrace].
	DefaultRunOnStartGrace = 5 * time.Minute
	// maxStaleRecoveryInterval caps the stale recovery ticker interval.
	maxStaleRecoveryInterval = 5 * time.Minute
	// minRunLease is the shortest run lease an instance persists, whatever
	// [WithStaleTaskTimeout] says. Leases are stored in whole seconds and renewed
	// every third of their length, so the floor keeps renewal latency headroom
	// above a second and bounds the heartbeat write rate.
	minRunLease = 5 * time.Second
)

type options struct {
	tickInterval     time.Duration `optgen:"default=DefaultTickInterval"`
	historyRetention time.Duration `optgen:"default=DefaultHistoryRetention"`
	staleTaskTimeout time.Duration `optgen:"default=DefaultStaleTaskTimeout"`
	cleanupInterval  time.Duration `optgen:"default=DefaultCleanupInterval"`

	// storageTimeout bounds every [Storage] call the scheduler issues from its
	// own goroutines — the tick loop, history cleanup, stale recovery, and the
	// bookkeeping writes that follow a task run. Those calls use the
	// scheduler's lifecycle context, which carries no deadline, so without this
	// cap a wedged backend stalls the single main loop indefinitely. Calls made
	// on a caller-supplied context (Register, PauseTask, TasksPaginated, …)
	// keep that caller's deadline instead.
	storageTimeout time.Duration `optgen:"default=DefaultStorageTimeout"`

	// runOnStartGrace dedupes RunOnStart across instances: when a periodic
	// task is registered again (an instance restarts), it runs at once only if
	// it has not started within this window, so a rolling restart of several
	// replicas triggers one run. Non-positive values keep the default; pass
	// time.Nanosecond to run on every start.
	runOnStartGrace time.Duration `optgen:"default=DefaultRunOnStartGrace"`

	// instanceID identifies this scheduler as the owner of the runs it
	// executes; it prefixes every run ID. It must be unique among the schedulers
	// running concurrently against one storage. Empty (the default) means a
	// random ID chosen by [New]. A stable value, such as a pod name, lets a
	// restarted process recover its own interrupted runs at once instead of
	// waiting for their leases to expire.
	instanceID string

	maxConcurrentTasks        int `optgen:"default=DefaultMaxConcurrentTasks"`
	reservedHighPrioritySlots int `optgen:"default=DefaultReservedHighPrioritySlots"`
	logger                    *slog.Logger
	leaderElector             LeaderElector                    `optgen:"notnil" optval:"nil"`
	concurrencyLimitFunc      concurrency.ConcurrencyLimitFunc `opt:"-"`
	collector                 metrics.Collector                `opt:"-"`
	readinessProbe            func() bool                      `opt:"-"`

	// historyStorage keeps execution history apart from the task state, in a
	// backend suited to an append-only log. Nil (the default) keeps history in
	// the [Storage] passed to [New].
	historyStorage HistoryStorage `optgen:"notnil" optval:"nil"`
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

// WithReadinessProbe sets a function that the scheduler evaluates at the start
// of each tick. If it returns false, the tick is skipped entirely — the main
// loop keeps running (so [Scheduler.Stop] works cleanly), but no tasks are
// dispatched. Once the probe returns true, normal dispatch proceeds.
//
// When no probe is configured (the default), the scheduler is always ready.
//
// This is useful for deferring task execution until all subsystems (databases,
// caches, message brokers, etc.) have finished initializing:
//
//	scheduler.WithReadinessProbe(func() bool {
//	    return healthCoordinator.CheckHealth(ctx) == health.StatusServing
//	})
func WithReadinessProbe(fn func() bool) Option {
	return func(o *options) {
		o.readinessProbe = fn
	}
}

// WithCollector sets the [metrics.Collector] used to record scheduler metrics.
// When nil (the default), [metrics.Noop] is used and all metric operations
// become zero-cost no-ops.
func WithCollector(c metrics.Collector) Option {
	return func(o *options) {
		o.collector = c
	}
}
