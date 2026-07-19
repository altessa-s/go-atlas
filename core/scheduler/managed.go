// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrSchedulerManaged is returned by [ManagedTask.Run] — and by the manual
// Run* entry points of subsystems built on it — when the task has been handed
// to a scheduler and direct invocation is no longer allowed.
var ErrSchedulerManaged = errors.New("function is managed by scheduler, direct calls not allowed")

// ManagedTask coordinates a background cycle that can be driven either by a
// [TaskRegistrar] or by manual calls, but never by both at once. It owns the
// two pieces of state every such cycle needs:
//
//   - a "registered" flag marking the cycle as scheduler-managed, after which
//     manual [ManagedTask.Run] calls fail with [ErrSchedulerManaged];
//   - a "running" single-flight guard that collapses overlapping invocations:
//     while one execution is in flight, subsequent ones return nil without
//     running the function.
//
// The zero value is ready to use. A ManagedTask must not be copied after
// first use. All methods are safe for concurrent use.
type ManagedTask struct {
	registered atomic.Bool
	running    atomic.Bool
}

// SchedulerFunc marks the task as scheduler-managed and returns fn wrapped
// with the single-flight guard, suitable for [TaskConfig.Func]. After the
// first call, [ManagedTask.Run] returns [ErrSchedulerManaged].
func (t *ManagedTask) SchedulerFunc(fn TaskFunc) TaskFunc {
	t.registered.Store(true)
	return func(ctx context.Context) error {
		return t.TryRun(ctx, fn)
	}
}

// Run executes fn with the single-flight guard unless the task is
// scheduler-managed, in which case it returns [ErrSchedulerManaged] without
// invoking fn. It is the entry point for manual, caller-driven cycles.
func (t *ManagedTask) Run(ctx context.Context, fn TaskFunc) error {
	if t.registered.Load() {
		return ErrSchedulerManaged
	}
	return t.TryRun(ctx, fn)
}

// TryRun executes fn with the single-flight guard but without the
// scheduler-managed check: if another execution is already in flight it
// returns nil without invoking fn. It is the entry point for internal
// drivers (poll loops, watchers) that share the guard with a scheduler.
func (t *ManagedTask) TryRun(ctx context.Context, fn TaskFunc) error {
	if !t.running.CompareAndSwap(false, true) {
		return nil // Already running, skip this cycle.
	}
	defer t.running.Store(false)
	return fn(ctx)
}

// MarkRegistered marks the task as scheduler-managed without wrapping a
// function. Use it when the guarded function was registered separately —
// e.g. via [ManagedTask.TryRun] inside a hand-built [TaskConfig.Func] — and
// the flag should flip only after a successful registration.
func (t *ManagedTask) MarkRegistered() {
	t.registered.Store(true)
}

// Registered reports whether the task has been marked scheduler-managed.
func (t *ManagedTask) Registered() bool {
	return t.registered.Load()
}
