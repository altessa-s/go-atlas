// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/leadelect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Scheduler manages periodic and one-shot task execution with persistent state.
// It provides task registration, status tracking, pause/resume/disable
// capabilities, execution history recording, and stale-task recovery.
//
// Each registered task runs in its own goroutine; the number of concurrent
// executions is governed by either a static limit ([WithMaxConcurrentTasks])
// or a dynamic limit function ([WithConcurrencyLimitFunc]). Long-running tasks
// do not block other tasks from being dispatched.
//
// Priority levels control dispatch order when concurrency is contended:
//   - [TaskPriorityCritical]: bypass all concurrency limits, execute immediately
//   - [TaskPriorityHigh]: use reserved slots + shared pool
//   - [TaskPriorityNormal]: use shared pool only (default)
//   - [TaskPriorityLow]: execute only when no higher-priority tasks are waiting
//
// For distributed environments, use [WithLeaderElector] to ensure only the
// leader node executes tasks.
//
// All exported methods are safe for concurrent use.
type Scheduler struct {
	storage      Storage
	opts         *options
	logger       *slog.Logger
	parser       cron.Parser
	filterParser *filter.Parser

	mu    sync.RWMutex
	tasks map[string]*registeredTask

	isRunning     atomic.Bool
	stopCtx       context.Context
	stopCtxCancel context.CancelFunc
	wg            sync.WaitGroup

	// semaphore limits concurrent task executions for Normal/Low priority.
	// nil means unlimited concurrency.
	semaphore chan struct{}
	// semaphoreUsed tracks current semaphore usage atomically for metrics.
	semaphoreUsed atomic.Int32

	// highPrioritySemaphore provides reserved slots for High priority tasks.
	// High priority tasks can use both this and the main semaphore.
	highPrioritySemaphore chan struct{}
	// highPrioritySemaphoreUsed tracks current high priority semaphore usage atomically.
	highPrioritySemaphoreUsed atomic.Int32

	// runningCount tracks the number of currently executing non-critical tasks
	// in dynamic concurrency mode and static unlimited mode. Used for
	// tick-gated dispatch and RunningTasksCount observability.
	runningCount atomic.Int32

	// leaderElector provides distributed leader election support.
	// If set, tasks only execute when this node is the leader.
	leaderElector leadelect.LeaderElector
}

// registeredTask holds the runtime state of a registered task.
type registeredTask struct {
	config  corescheduler.TaskConfig
	running atomic.Bool
}

// pendingTask holds task info for priority sorting.
type pendingTask struct {
	task  *registeredTask
	state *TaskState
}

// New creates a new [Scheduler] backed by the provided [Storage] implementation.
// The scheduler is created in a stopped state; call [Scheduler.Start] to begin
// the main loop and [Scheduler.Stop] to shut it down gracefully.
//
// When [WithConcurrencyLimitFunc] is provided, the scheduler operates in dynamic
// concurrency mode and does not allocate semaphores. Otherwise, if
// [WithMaxConcurrentTasks] is positive, static semaphore-based concurrency
// limiting is used.
//
// Example:
//
//	s := scheduler.New(storage, scheduler.WithTickInterval(time.Second))
//	if err := s.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
func New(storage Storage, opts ...Option) *Scheduler {
	o := newOptions(opts...)

	fp, _ := filter.NewParser() //nolint:errcheck // parser init never fails with no options

	s := &Scheduler{
		storage:       storage,
		opts:          o,
		logger:        o.logger,
		tasks:         make(map[string]*registeredTask),
		leaderElector: o.leaderElector,
		filterParser:  fp,
		parser: cron.NewParser(
			cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		),
	}

	// Dynamic concurrency mode: no semaphores needed, limits are evaluated per tick.
	if o.concurrencyLimitFunc != nil {
		return s
	}

	// Static mode: initialize semaphores for concurrency limiting if configured.
	if o.maxConcurrentTasks > 0 {
		// Main semaphore for Normal/Low tasks
		// Reserve some slots for High priority
		normalSlots := o.maxConcurrentTasks - o.reservedHighPrioritySlots
		if normalSlots < 1 {
			normalSlots = 1
		}
		s.semaphore = make(chan struct{}, normalSlots)

		// Reserved semaphore for High priority tasks
		if o.reservedHighPrioritySlots > 0 {
			s.highPrioritySemaphore = make(chan struct{}, o.reservedHighPrioritySlots)
		}
	}

	return s
}

// Start begins the scheduler's main loop. It loads existing task states from
// storage, recovers any tasks left in [TaskStatusRunning] from a previous crash,
// and starts the tick, cleanup, and stale-recovery goroutines.
//
// Start must be called exactly once. Calling Start on an already-running
// scheduler returns an error. The provided context is used only for the initial
// load; the scheduler's internal lifecycle is governed by the context derived
// inside Start and canceled by [Scheduler.Stop].
func (s *Scheduler) Start(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(false, true) {
		return errors.New("scheduler already running")
	}

	s.stopCtx, s.stopCtxCancel = context.WithCancel(ctx)

	// Load existing tasks from storage
	if err := s.loadTasks(ctx); err != nil {
		s.isRunning.Store(false)
		return coreerrs.WrapOperation(err, "load tasks")
	}

	// Recover any tasks left in Running status from a previous crash
	s.recoverStaleTasks(ctx, true)

	s.wg.Go(s.run)

	logAttrs := []any{
		slog.Duration("tick_interval", s.opts.tickInterval),
		slog.Duration("stale_task_timeout", s.opts.staleTaskTimeout),
	}
	switch {
	case s.opts.concurrencyLimitFunc != nil:
		logAttrs = append(logAttrs,
			slog.String("concurrency_mode", "dynamic"),
			slog.Int("current_limit", s.opts.concurrencyLimitFunc()),
			slog.Int("reserved_high_priority_slots", s.opts.reservedHighPrioritySlots))
	case s.opts.maxConcurrentTasks > 0:
		logAttrs = append(logAttrs,
			slog.Int("max_concurrent_tasks", s.opts.maxConcurrentTasks),
			slog.Int("reserved_high_priority_slots", s.opts.reservedHighPrioritySlots))
	default:
		logAttrs = append(logAttrs, slog.String("max_concurrent_tasks", "unlimited"))
	}
	if s.leaderElector != nil {
		logAttrs = append(logAttrs, slog.String("leader_election", "enabled"))
	}
	s.logger.InfoContext(ctx, "scheduler started", logAttrs...)

	return nil
}

// Stop gracefully stops the scheduler by canceling the internal context and
// waiting for all in-flight task goroutines to finish. If the provided context
// expires before all tasks complete, Stop returns the context error and some
// tasks may still be running in the background.
//
// Calling Stop on an already-stopped scheduler is a no-op and returns nil.
// Stop must be called after [Scheduler.Start].
func (s *Scheduler) Stop(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(true, false) {
		return nil
	}

	s.stopCtxCancel()

	// Wait for the main loop and all tasks to complete
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.InfoContext(ctx, "scheduler stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.WarnContext(ctx, "scheduler stop timed out, some tasks may still be running")
		return ctx.Err()
	}
}

// IsRunning reports whether the scheduler's main loop is active. It returns
// true between a successful [Scheduler.Start] and the completion of
// [Scheduler.Stop]. Safe for concurrent use.
func (s *Scheduler) IsRunning() bool {
	return s.isRunning.Load()
}

// IsLeader reports whether this scheduler instance is the elected leader.
// Always returns true when no [WithLeaderElector] option was provided.
// Safe for concurrent use.
func (s *Scheduler) IsLeader() bool {
	if s.leaderElector == nil {
		return true
	}
	return s.leaderElector.IsLeader()
}

// RunningTasksCount returns the number of currently executing non-critical tasks.
//
// In dynamic concurrency mode and static unlimited mode, it returns the atomic
// running count. In static semaphore mode, it returns the sum of shared-pool
// and reserved high-priority semaphore usage. [TaskPriorityCritical] tasks are
// never counted because they bypass concurrency limits.
//
// Safe for concurrent use.
func (s *Scheduler) RunningTasksCount() int {
	if s.opts.concurrencyLimitFunc != nil || s.semaphore == nil {
		return int(s.runningCount.Load())
	}
	count := int(s.semaphoreUsed.Load())
	count += int(s.highPrioritySemaphoreUsed.Load())
	return count
}

// AvailableSlots returns the number of concurrency slots available for task
// dispatch, including both shared-pool and reserved high-priority slots.
//
// In dynamic mode, it evaluates the concurrency limit function and subtracts
// the current running count. In static mode, it sums the free capacity of both
// semaphores. Returns -1 when concurrency limiting is not enabled (unlimited).
//
// Safe for concurrent use.
func (s *Scheduler) AvailableSlots() int {
	if s.opts.concurrencyLimitFunc != nil {
		limit := s.opts.concurrencyLimitFunc()
		if limit <= 0 {
			return -1
		}
		return max(0, limit-int(s.runningCount.Load()))
	}
	if s.semaphore == nil {
		return -1 // Unlimited
	}
	available := cap(s.semaphore) - int(s.semaphoreUsed.Load())
	if s.highPrioritySemaphore != nil {
		available += cap(s.highPrioritySemaphore) - int(s.highPrioritySemaphoreUsed.Load())
	}
	return available
}

// AvailableHighPrioritySlots returns the number of reserved slots currently
// available for [TaskPriorityHigh] tasks.
//
// In dynamic concurrency mode, reserved slots are enforced at dispatch time
// rather than tracked via a semaphore, so this method returns -1. It also
// returns -1 when concurrency limiting is not enabled or no reserved slots are
// configured.
//
// Safe for concurrent use.
func (s *Scheduler) AvailableHighPrioritySlots() int {
	if s.opts.concurrencyLimitFunc != nil {
		return -1
	}
	if s.highPrioritySemaphore == nil {
		return -1
	}
	return cap(s.highPrioritySemaphore) - int(s.highPrioritySemaphoreUsed.Load())
}
