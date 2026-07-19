// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"iter"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/data/probfilter"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// registerUpdateTask registers the update cycle task with the scheduler if configured.
func (t *Manager[T]) registerUpdateTask(opts *options) error {
	if nilcheck.IsNil(t.scheduler) || opts.updateSchedule == "" {
		return nil
	}

	// Skip registration for static providers
	if static, ok := t.secretStorage.(Static); ok && static.IsStatic() {
		return nil
	}

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:          "secrets-cache-sync",
		Description: "Synchronize secrets cache with storage",
		Func:        t.RegisterUpdateCycleSchedulerFunc(),
		Schedule:    opts.updateSchedule,
		RunOnStart:  opts.runOnStart,
		Priority:    corescheduler.TaskPriorityNormal,
	}

	if err := t.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register secrets update cycle task")
	}

	return nil
}

// RegisterUpdateCycleSchedulerFunc returns a function for use by a scheduler and marks
// update cycle as scheduler-managed. After calling this method, direct calls to
// RunUpdateCycle will return [corescheduler.ErrSchedulerManaged].
func (t *Manager[T]) RegisterUpdateCycleSchedulerFunc() func(context.Context) error {
	return t.updateCycleTask.SchedulerFunc(t.runUpdateCycleInternal)
}

// RunUpdateCycle executes a single cache synchronization cycle.
// This method is designed to be called manually for one-time synchronization.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (t *Manager[T]) RunUpdateCycle(ctx context.Context) error {
	return t.updateCycleTask.Run(ctx, t.runUpdateCycleInternal)
}

// runUpdateCycleInternal executes the actual cache synchronization cycle.
// This method fetches all secrets from storage and updates the cache accordingly:
//   - Adds new secrets to cache
//   - Updates existing secrets if version has changed
//   - Removes cached secrets that no longer exist in storage
//
// The operation includes retry logic with exponential backoff for storage failures.
// Cache operations use the LRU eviction policy to maintain the configured size limit.
// Callers must route through updateCycleTask so overlapping cycles collapse
// into a single execution.
//
// Returns nil on success, or an error if the storage operation fails after retries.
func (t *Manager[T]) runUpdateCycleInternal(ctx context.Context) error {
	stop := t.metrics.updateCycleDuration.Start()
	defer stop()

	var list []*Value[T]

	// Apply timeout to the context if not already set
	ctx, cancel := corecontext.ApplyTimeout(ctx, DefaultOperationsTimeout)
	defer cancel()

	// Retry logic for List operation.
	err := coreretry.Do(ctx, func(ctx context.Context) error {
		var err error
		list, err = t.secretStorage.List(ctx)
		return err
	},
		coreretry.WithMaxAttempts(t.opts.maxRetries),
		coreretry.WithNextDelay(coreretry.Exponential(t.opts.exponentialConfig)),
	)

	if err != nil {
		t.opts.logger.ErrorContext(ctx, "failed to list secrets from storage", slogx.Error(err))
		t.metrics.updateCycleErrors.Inc()
		return err
	}

	// Rebuild negative filter if configured
	if t.negativeFilter != nil {
		if rebuilder, ok := t.negativeFilter.(interface {
			Rebuild(context.Context, probfilter.DataLoader) error
		}); ok {
			loader := probfilter.NewDataLoader(func() iter.Seq[string] {
				return func(yield func(string) bool) {
					for _, v := range list {
						if !yield(v.Key) {
							return
						}
					}
				}
			}, probfilter.WithCount(int64(len(list))))

			if err := rebuilder.Rebuild(ctx, loader); err != nil {
				t.opts.logger.ErrorContext(ctx, "failed to rebuild negative filter", slog.Any("error", err))
			}
		}
	}

	if len(list) == 0 {
		t.opts.logger.DebugContext(ctx, "no values found")
		return nil
	}

	// Build set of current keys from storage using pool
	newKeys := keySetPool.GetWithCapacity(len(list))
	defer keySetPool.Put(newKeys)

	for _, val := range list {
		(*newKeys)[val.Key] = struct{}{}
	}

	// Find deleted keys by checking current cache contents
	// Use string slice pool to avoid allocations
	cacheKeysSlice := corestrings.GetStringSliceWithCapacity(t.cache.Len())
	defer corestrings.PutStringSlice(cacheKeysSlice)

	deletedCount := 0
	for key, value := range t.cache.All() {
		*cacheKeysSlice = append(*cacheKeysSlice, key)
		if _, exists := (*newKeys)[key]; !exists {
			// Securely clear the value before removing from cache
			value.Clear()
			t.cache.Remove(key)
			deletedCount++
		}
	}

	// Process new/updated values
	updatedCount := 0
	for _, val := range list {
		existing, exists := t.cache.Get(val.Key)
		if !exists || existing.Version != val.Version {
			t.cache.Put(val.Key, val) // Cache handles eviction automatically
			updatedCount++
		}
	}

	t.lastUpdateTime.Store(time.Now())
	t.metrics.cacheSize.Set(float64(t.cache.Len()))

	t.opts.logger.DebugContext(ctx, "values updated",
		slog.Int("secrets_count", len(list)),
		slog.Int("updated_count", updatedCount),
		slog.Int("deleted_count", deletedCount),
		slog.Int("cache_size", t.cache.Len()))

	// Notify watch manager about changes
	if t.watchManager != nil {
		t.watchManager.notifyChanges(ctx, list)
	}

	return nil
}
