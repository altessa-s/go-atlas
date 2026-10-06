// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/data/probfilter"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
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
// Keys the Manager cached or deleted after the cycle began (Save, a forced
// Value fetch, WarmCache, Delete) are left alone: the storage snapshot may
// predate those writes, so the cycle neither evicts and clears, overwrites,
// nor re-inserts them; the next cycle reconciles them. A bounded cache may
// still drop such an entry by capacity when the cycle inserts another key,
// as any insertion can; that never clears the value.
//
// A successful listing without any secret leaves the cache as it is until
// WithEmptyListingThreshold consecutive cycles (default
// [DefaultEmptyListingThreshold]) list nothing, so a transiently empty listing
// cannot wipe the cache while a provider whose last secret was deleted still
// stops serving it eventually.
//
// Cached values and the values passed to watchers are Manager-owned copies;
// the instances the provider returned are never cached or cleared.
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

	// Apply timeout to the context if not already set
	ctx, cancel := corecontext.ApplyTimeout(ctx, DefaultOperationsTimeout)
	defer cancel()

	// Track cache writes from before the storage is listed until the cycle ends.
	t.beginCycle()
	defer t.endCycle()

	list, err := t.listAndRebuildNegativeFilter(ctx)
	if err != nil {
		t.opts.logger.ErrorContext(ctx, "failed to list secrets from storage", slogx.Error(err))
		t.metrics.updateCycleErrors.Inc()
		return err
	}

	if len(list) == 0 {
		empty := int(t.emptyListings.Add(1))
		if empty < t.opts.emptyListingThreshold {
			t.opts.logger.DebugContext(ctx, "no values found; cache kept",
				slog.Int("empty_listings", empty), slog.Int("threshold", t.opts.emptyListingThreshold))
			return nil
		}
		t.opts.logger.WarnContext(ctx, "storage listed no secrets in consecutive cycles; clearing the cache",
			slog.Int("empty_listings", empty))
	} else {
		t.emptyListings.Store(0)
	}

	// Build set of current keys from storage using pool
	newKeys := keySetPool.GetWithCapacity(len(list))
	defer keySetPool.Put(newKeys)

	// Values the Manager cannot copy safely are neither cached nor sent to
	// watchers; their keys still count as listed, so a cached older version
	// is not evicted.
	var rejected map[string]struct{}
	for _, v := range list {
		(*newKeys)[v.Key] = struct{}{}
		if err := t.checkPayload(v.Value); err != nil {
			t.opts.logger.ErrorContext(ctx, "listed value not cached", slogx.Error(err), slog.String("key", v.Key))
			if rejected == nil {
				rejected = make(map[string]struct{})
			}
			rejected[v.Key] = struct{}{}
		}
	}

	// Watchers get their own copies, so notification never reads an
	// instance the cache or the provider holds.
	notify := t.watchManager != nil && t.watchManager.hasWatchers()
	var current []*Value[T]
	if notify {
		current = make([]*Value[T], 0, len(list))
		for _, v := range list {
			if _, bad := rejected[v.Key]; !bad {
				current = append(current, v.clone())
			}
		}
	}
	deletedCount, updatedCount := 0, 0

	t.cacheMu.Lock()
	// Find deleted keys by checking current cache contents
	for key, value := range t.cache.All() {
		if _, exists := (*newKeys)[key]; exists || t.isDirtyLocked(key) {
			continue
		}
		// Securely clear the value before removing from cache
		value.Clear()
		t.cache.Remove(key)
		deletedCount++
	}

	// Process new/updated values
	for _, v := range list {
		if _, bad := rejected[v.Key]; bad || t.isDirtyLocked(v.Key) {
			continue
		}
		existing, exists := t.cache.Get(v.Key)
		if !exists || existing.Version != v.Version {
			// Cache a Manager-owned copy; the provider may keep or share
			// the listed instance. Cache handles eviction automatically.
			t.cache.Put(v.Key, v.clone())
			updatedCount++
		}
	}
	cacheSize := t.cache.Len()
	t.cacheMu.Unlock()

	t.lastUpdateTime.Store(time.Now())
	t.metrics.cacheSize.Set(float64(cacheSize))

	t.opts.logger.DebugContext(ctx, "values updated",
		slog.Int("secrets_count", len(list)),
		slog.Int("updated_count", updatedCount),
		slog.Int("deleted_count", deletedCount),
		slog.Int("cache_size", cacheSize))

	// Notify watch manager about changes
	if notify {
		t.watchManager.notifyChanges(ctx, current, rejected)
	}

	return nil
}

// beginCycle starts tracking the keys the Manager caches or deletes while an
// update cycle runs.
func (t *Manager[T]) beginCycle() {
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	t.cycleDirty = make(map[string]struct{})
}

// endCycle stops tracking cache writes for the update cycle.
func (t *Manager[T]) endCycle() {
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	t.cycleDirty = nil
}

// isDirtyLocked reports whether key was cached or deleted since the running
// update cycle began. The caller holds cacheMu.
func (t *Manager[T]) isDirtyLocked(key string) bool {
	_, dirty := t.cycleDirty[key]
	return dirty
}

// listSnapshot lists all secrets from storage (with retries), dropping nil
// values and values without a key. The listed instances belong to the
// provider: the Manager only reads and copies them, never caches or clears
// them.
func (t *Manager[T]) listSnapshot(ctx context.Context) ([]*Value[T], error) {
	list, err := t.listWithRetry(ctx)
	if err != nil {
		return nil, err
	}
	// A new slice: the provider may share the one it returned.
	listed := make([]*Value[T], 0, len(list))
	for _, v := range list {
		if v != nil && v.Key != "" {
			listed = append(listed, v)
		}
	}
	return listed, nil
}

// listAndRebuildNegativeFilter lists all secrets from storage (with retries)
// and, when the negative filter is rebuildable, rebuilds it from that list.
//
// The list is taken inside the rebuild's data loader, i.e. after the filter
// started journaling concurrent adds: a Save that persists a key after the
// snapshot adds it to the filter while the rebuild journals it, so the
// rebuilt filter cannot lose it. A rebuild failure is logged; a list failure
// is returned.
func (t *Manager[T]) listAndRebuildNegativeFilter(ctx context.Context) ([]*Value[T], error) {
	rebuilder, ok := t.negativeFilter.(probfilter.RebuildableFilter)
	if t.negativeFilter == nil || !ok {
		return t.listSnapshot(ctx)
	}

	var (
		list    []*Value[T]
		listErr error
		listed  bool
	)
	loader := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			list, listErr = t.listSnapshot(ctx)
			listed = true
			if listErr != nil {
				yield("", listErr)
				return
			}
			for _, v := range list {
				if !yield(v.Key, nil) {
					return
				}
			}
		}
	})

	rebuildErr := rebuilder.Rebuild(ctx, loader)
	switch {
	case !listed:
		// The rebuild ended before loading (e.g. a closed filter, or a
		// shared filter another process is rebuilding).
		if rebuildErr != nil {
			t.logRebuildError(ctx, rebuildErr)
		}
		return t.listSnapshot(ctx)
	case listErr != nil:
		return nil, listErr
	case rebuildErr != nil:
		t.logRebuildError(ctx, rebuildErr)
	}
	return list, nil
}

// logRebuildError logs a failed negative-filter rebuild. A rebuild skipped
// because another process holds the shared filter's rebuild lease is
// expected on every node but one and is logged at debug level only.
func (t *Manager[T]) logRebuildError(ctx context.Context, err error) {
	if errors.Is(err, probfilter.ErrRebuildInProgress) {
		t.opts.logger.DebugContext(ctx, "negative filter rebuild skipped: rebuild in progress elsewhere", slog.Any("error", err))
		return
	}
	t.opts.logger.ErrorContext(ctx, "failed to rebuild negative filter", slog.Any("error", err))
}

// listWithRetry lists all secrets from storage with exponential backoff.
func (t *Manager[T]) listWithRetry(ctx context.Context) ([]*Value[T], error) {
	var list []*Value[T]
	err := coreretry.Do(ctx, func(ctx context.Context) error {
		var err error
		list, err = t.secretStorage.List(ctx)
		return err
	},
		coreretry.WithMaxAttempts(t.opts.maxRetries),
		coreretry.WithNextDelay(coreretry.Exponential(t.opts.exponentialConfig)),
	)
	return list, err
}
