// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"log/slog"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// registerSchedulerTask registers the health check task with the scheduler if configured.
func (c *Coordinator) registerSchedulerTask(opts *options) error {
	if c.scheduler == nil || opts.checkSchedule == "" {
		return nil
	}

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:             "health-check",
		Description:    "Periodic health check cycle for all watched services",
		Func:           c.RegisterHealthCheckSchedulerFunc(),
		Schedule:       opts.checkSchedule,
		Priority:       corescheduler.TaskPriorityNormal,
		RunOnStart:     true,
		Unmanaged:      true,
		DisableHistory: true,
	}

	if err := c.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register health check task")
	}

	c.logger.Debug("registered health check task",
		slog.String("schedule", opts.checkSchedule))

	return nil
}

// RegisterHealthCheckSchedulerFunc returns a function for use by a scheduler and marks
// health check as scheduler-managed. After calling this method, direct calls to
// RunHealthCheckCycle will return ErrSchedulerManaged.
func (c *Coordinator) RegisterHealthCheckSchedulerFunc() func(context.Context) error {
	c.schedulerHealthCheckRegistered.Store(true)
	return c.runHealthCheckCycleInternal
}

// RunHealthCheckCycle performs a single health check cycle for all watched services.
// This method is designed to be called manually for one-time health check.
// If the function is registered with a scheduler, this method returns ErrSchedulerManaged.
func (c *Coordinator) RunHealthCheckCycle(ctx context.Context) error {
	if c.schedulerHealthCheckRegistered.Load() {
		return ErrSchedulerManaged
	}
	return c.runHealthCheckCycleInternal(ctx)
}

// runHealthCheckCycleInternal performs the actual health check cycle.
// It is safe to call concurrently; if already running, returns immediately.
func (c *Coordinator) runHealthCheckCycleInternal(ctx context.Context) error {
	// Prevent concurrent execution
	if !c.healthCheckRunning.CompareAndSwap(false, true) {
		return nil // Already running, skip this cycle
	}
	defer c.healthCheckRunning.Store(false)

	if c.closed.Load() {
		return nil
	}

	// Collect all services with active watchers
	watchedServices := make(map[string]struct{})
	for i := range c.watcherShards {
		shard := &c.watcherShards[i]
		shard.mu.RLock()
		for service := range shard.watchers {
			watchedServices[service] = struct{}{}
		}
		shard.mu.RUnlock()
	}

	if len(watchedServices) == 0 {
		return nil
	}

	// Check health for each watched service and notify watchers if changed
	for service := range watchedServices {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		checkCtx, cancel := context.WithTimeout(ctx, c.checkTimeout)
		currentStatus := c.getHealthStatus(checkCtx, service)
		cancel()

		// Find watchers for this service and notify if status changed
		shardIdx := c.getShardIndex(service)
		shard := &c.watcherShards[shardIdx]

		shard.mu.RLock()
		watchers := shard.watchers[service]
		if watchers == nil {
			shard.mu.RUnlock()
			continue
		}

		// Collect watchers that need notification
		var toNotify []*watcher
		for w := range watchers {
			if w.lastStatus != currentStatus {
				toNotify = append(toNotify, w)
			}
		}
		shard.mu.RUnlock()

		// Notify watchers outside the lock
		for _, w := range toNotify {
			c.logger.Info("health status changed",
				slog.String("service", service),
				slog.String("from", w.lastStatus.String()),
				slog.String("to", currentStatus.String()))
			w.lastStatus = currentStatus
			w.notify(currentStatus)
		}
	}

	return nil
}
