// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

var (
	// ErrWatcherClosed indicates that the watcher has been closed
	ErrWatcherClosed = errors.New("watcher is closed")
)

// watchManager manages watch operations and event dispatching for secret changes.
// It coordinates between the Manager's cache updates and active watch instances,
// ensuring that all watchers receive appropriate events when secrets are created,
// updated, or deleted. The watchManager is thread-safe and handles concurrent
// access from multiple goroutines.
type watchManager[T any] struct {
	providerName string

	// logger for watch operations
	logger *slog.Logger

	// manager is the parent Manager instance that owns this watchManager
	manager *Manager[T]

	// mu protects watchers map and closed flag
	mu sync.RWMutex

	// watchers holds all active watch operations
	watchers map[string]*watchInstance[T]

	// watcherCounter generates unique IDs for watch instances
	watcherCounter atomic.Uint64

	// closed indicates if the manager is closed
	closed bool

	// snapshotMapPool is a pool for reusing snapshot maps during notifyChanges
	snapshotMapPool *coremaps.Pool[string, *Value[T]]
}

// watchInstance represents a single watch operation with its own event channel,
// filters, and snapshot tracking. Each instance operates independently and
// maintains its own view of secret changes for event generation.
type watchInstance[T any] struct {
	id                  string
	ctx                 context.Context // context for this watch instance
	opts                WatchOptions
	events              chan WatchEvent[T]
	done                chan struct{}
	cancel              context.CancelFunc
	filters             []WatchFilter[T]
	snapshotMu          sync.RWMutex         // protects lastSnapshot
	lastSnapshot        map[string]*Value[T] // per-instance snapshot
	closeOnce           sync.Once            // ensure channels are closed only once
	closed              atomic.Bool          // indicates if instance is closed
	initialSnapshotDone atomic.Bool          // true after first snapshot is populated
}

// newWatchManager creates a new watch manager for the specified provider.
// The watch manager coordinates event dispatching between the Manager's cache
// updates and active watch instances. It uses the provided logger for operational
// logging and debugging information.
func newWatchManager[T any](manager *Manager[T], providerName string, logger *slog.Logger) *watchManager[T] {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &watchManager[T]{
		providerName:    providerName,
		logger:          logger,
		manager:         manager,
		watchers:        make(map[string]*watchInstance[T]),
		snapshotMapPool: coremaps.NewPool[string, *Value[T]](0), // Use default capacity
	}
}

// Watch starts a new watch operation for secret changes with the specified options.
// It creates a new watch instance with its own event channel and filter configuration,
// then registers it with the watch manager for event dispatching. The watch operation
// continues until the context is canceled or Stop() is called on the returned WatchResult.
//
// The watch operation receives events generated during Manager cache updates, providing
// a unified event stream regardless of the underlying storage provider type.
//
// Returns a WatchResult containing the event channel and control functions, or an error
// if the watch operation cannot be started (e.g., if the watch manager is closed).
func (wm *watchManager[T]) Watch(ctx context.Context, opts WatchOptions) (*WatchResult[T], error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.closed {
		return nil, ErrWatcherClosed
	}

	// Apply defaults
	if opts.BufferSize <= 0 {
		opts.BufferSize = 100
	}

	// Create watch instance
	watcherID := wm.generateWatcherID()
	watchCtx, cancel := context.WithCancel(ctx)

	instance := &watchInstance[T]{
		id:           watcherID,
		ctx:          watchCtx,
		opts:         opts,
		events:       make(chan WatchEvent[T], opts.BufferSize),
		done:         make(chan struct{}),
		cancel:       cancel,
		lastSnapshot: make(map[string]*Value[T]),
	}

	// Setup filters
	instance.setupFilters()

	// Register watcher
	wm.watchers[watcherID] = instance

	// Start watching - simple goroutine that waits for context cancellation
	go func() {
		defer panics.Handle(watchCtx)
		defer func() {
			instance.closed.Store(true)
			instance.closeOnce.Do(func() {
				close(instance.events)
				close(instance.done)
			})
			wm.removeWatcher(instance.id)
			wm.logger.DebugContext(watchCtx, "watch stopped", slog.String("watcher_id", instance.id))
		}()

		wm.logger.DebugContext(watchCtx, "starting event-driven watcher",
			slog.String("watcher_id", instance.id))

		// Wait for context cancellation - updates come from Manager.updateValues()
		<-watchCtx.Done()
	}()

	// Create result
	result := &WatchResult[T]{
		Events: instance.events,
		Done:   instance.done,
		Stop: func() {
			wm.stopWatcher(watcherID)
		},
	}

	wm.logger.DebugContext(ctx, "watch started",
		slog.String("watcher_id", watcherID),
		slog.Int("key_count", len(opts.Keys)),
		slog.Int("buffer_size", opts.BufferSize))

	return result, nil
}

// setupFilters configures event filters for the watch instance
func (wi *watchInstance[T]) setupFilters() {
	wi.filters = coreslices.AppendIfFunc(wi.filters, len(wi.opts.Keys) > 0, func() []WatchFilter[T] {
		return []WatchFilter[T]{FilterByKeys[T](wi.opts.Keys...)}
	})

	wi.filters = coreslices.AppendIfFunc(wi.filters, len(wi.opts.EventTypes) > 0, func() []WatchFilter[T] {
		return []WatchFilter[T]{FilterByEventTypes[T](wi.opts.EventTypes...)}
	})
}

// notifyChanges processes changes from Manager's updateValues() and generates events for all watchers.
// The updateCtx parameter is the context from the update operation, used for cancellation coordination.
func (wm *watchManager[T]) notifyChanges(updateCtx context.Context, currentList []*Value[T]) {
	// Fast path check with lock
	wm.mu.RLock()
	hasWatchers := len(wm.watchers) > 0
	wm.mu.RUnlock()
	if !hasWatchers {
		return // No active watchers
	}

	// Get a pooled map for the current values
	currentMapPtr := wm.snapshotMapPool.GetWithCapacity(len(currentList))
	defer wm.snapshotMapPool.Put(currentMapPtr)
	currentMap := *currentMapPtr

	// Populate the map from the list
	for _, value := range currentList {
		currentMap[value.Key] = value
	}

	// Copy watchers to avoid holding lock during event generation
	wm.mu.RLock()
	watchers := slices.Collect(maps.Values(wm.watchers))
	wm.mu.RUnlock()

	// Generate events for each active watcher (without holding lock)
	for _, instance := range watchers {
		// Check if update context is canceled (timeout exceeded)
		if updateCtx.Err() != nil {
			wm.logger.WarnContext(updateCtx, "update context canceled, stopping event dispatch",
				slog.Int("remaining_watchers", len(watchers)))
			return
		}

		// Check if watcher is still active and not closed
		if instance.closed.Load() {
			continue // Skip closed watchers
		}

		wm.mu.RLock()
		_, stillActive := wm.watchers[instance.id]
		wm.mu.RUnlock()

		if !stillActive {
			continue // Skip inactive watchers
		}

		// Prepare new snapshot OUTSIDE the lock to minimize critical section.
		newSnapshot := make(map[string]*Value[T], len(currentMap))
		maps.Copy(newSnapshot, currentMap)

		// Lock snapshot only for the pointer swap (O(1) operation).
		instance.snapshotMu.Lock()
		previous := instance.lastSnapshot
		isFirstSnapshot := !instance.initialSnapshotDone.Load()
		instance.lastSnapshot = newSnapshot
		instance.snapshotMu.Unlock()

		// Mark initial snapshot as done
		if isFirstSnapshot {
			instance.initialSnapshotDone.Store(true)

			// If SkipInitialEvents is enabled, don't generate events for first snapshot
			if instance.opts.SkipInitialEvents {
				//nolint:contextcheck // instance.ctx is the watcher's lifecycle context, not inherited
				wm.logger.DebugContext(instance.ctx, "skipping initial events",
					slog.String("watcher_id", instance.id),
					slog.Int("secrets_count", len(currentMap)))
				continue
			}
		}

		// Generate events for this watcher
		wm.generateChangeEvents(updateCtx, instance, previous, currentMap)
	}
}

// generateChangeEvents compares snapshots and generates appropriate events.
// Events are generated in deterministic order: created/updated events sorted by key,
// followed by deleted events sorted by key.
// The updateCtx is checked for cancellation to abort early if the update operation times out.
func (wm *watchManager[T]) generateChangeEvents(updateCtx context.Context, instance *watchInstance[T], previous, current map[string]*Value[T]) {
	now := time.Now()

	// Get sorted keys for deterministic event ordering
	currentKeys := slices.Sorted(maps.Keys(current))
	previousKeys := slices.Sorted(maps.Keys(previous))

	// Check for created and updated secrets (in sorted key order)
	for _, key := range currentKeys {
		// Check for update context cancellation
		if updateCtx.Err() != nil {
			return
		}

		currentValue := current[key]
		previousValue, existed := previous[key]

		var event WatchEvent[T]
		event.Key = key
		event.Value = currentValue
		event.OccurredTime = now
		event.Source = wm.providerName

		switch {
		case !existed:
			// New secret created
			event.Type = EventTypeCreated
		case currentValue.Version != previousValue.Version:
			// Secret updated
			event.Type = EventTypeUpdated
		default:
			// No change, skip
			continue
		}

		// Dispatch event if it passes filters
		if wm.shouldDispatchEvent(instance, event) {
			wm.safeEventSend(instance, event, key)
		}
	}

	// Check for deleted secrets (in sorted key order)
	for _, key := range previousKeys {
		// Check for update context cancellation
		if updateCtx.Err() != nil {
			return
		}

		if _, exists := current[key]; !exists {
			event := WatchEvent[T]{
				Type:          EventTypeDeleted,
				Key:           key,
				OccurredTime:  now,
				Source:        wm.providerName,
				PreviousValue: previous[key],
			}

			if wm.shouldDispatchEvent(instance, event) {
				wm.safeEventSend(instance, event, key)
			}
		}
	}
}

// shouldDispatchEvent checks if an event should be sent to the watcher
func (wm *watchManager[T]) shouldDispatchEvent(instance *watchInstance[T], event WatchEvent[T]) bool {
	// Check filters
	for _, filter := range instance.filters {
		if filter != nil && !filter(event) {
			return false
		}
	}

	return true
}

// safeEventSend safely sends an event to a watcher instance, handling closed channels
// and applying the configured buffer overflow policy.
func (wm *watchManager[T]) safeEventSend(instance *watchInstance[T], event WatchEvent[T], key string) {
	if instance.closed.Load() {
		return // Instance is already closed
	}

	// Channel was closed during send, log and continue
	logChannelClosed := func() {
		wm.logger.DebugContext(instance.ctx, "channel closed during event send",
			slog.String("watcher_id", instance.id),
			slog.String("event_type", event.Type.String()),
			slog.String("key", key))
	}

	switch instance.opts.BufferOverflowPolicy {
	case OverflowPolicyBlock:
		// Block until space is available or context is canceled
		sent, chClosed := panics.TrySend(instance.ctx, instance.events, event)
		switch {
		case chClosed:
			logChannelClosed()
		case !sent:
			wm.logger.DebugContext(instance.ctx, "context canceled while blocking on event send",
				slog.String("watcher_id", instance.id),
				slog.String("event_type", event.Type.String()),
				slog.String("key", key))
		}

	case OverflowPolicyDropOldest:
		// Try non-blocking send first
		sent, chClosed := panics.TrySendNonBlocking(instance.events, event)
		if chClosed {
			logChannelClosed()
			return
		}
		if sent {
			return // Event sent successfully
		}
		// Channel full - drop oldest and retry

		// Drop oldest event(s) until we can send
		for {
			select {
			case <-instance.events:
				// Dropped oldest event, try to send again
				sent, chClosed = panics.TrySendNonBlocking(instance.events, event)
				if chClosed {
					logChannelClosed()
					return
				}
				if sent {
					wm.logger.DebugContext(instance.ctx, "dropped oldest event to make room",
						slog.String("watcher_id", instance.id),
						slog.String("event_type", event.Type.String()),
						slog.String("key", key))
					return
				}
				// Still full (race condition), continue dropping
			default:
				// Channel is empty now but still can't send (shouldn't happen)
				// Fall back to blocking send
				if _, chClosed = panics.TrySend(instance.ctx, instance.events, event); chClosed {
					logChannelClosed()
				}
				return
			}
		}

	default: // OverflowPolicyDropNewest (default)
		sent, chClosed := panics.TrySendNonBlocking(instance.events, event)
		if chClosed {
			logChannelClosed()
			return
		}
		if !sent {
			// Channel full, drop this (newest) event
			wm.logger.DebugContext(instance.ctx, "event dropped (buffer full, policy: drop_newest)",
				slog.String("watcher_id", instance.id),
				slog.String("event_type", event.Type.String()),
				slog.String("key", key))
		}
	}
}

// stopWatcher stops a specific watch operation
func (wm *watchManager[T]) stopWatcher(watcherID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if instance, exists := wm.watchers[watcherID]; exists {
		instance.closed.Store(true) // Mark as closed BEFORE cancel to prevent event dispatch race
		instance.cancel()
		delete(wm.watchers, watcherID)
	}
}

// removeWatcher removes a watcher from the active list
func (wm *watchManager[T]) removeWatcher(watcherID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	delete(wm.watchers, watcherID)
}

// close stops all watch operations and closes the manager.
// This is an internal method called from Manager.Shutdown().
// For explicit timeout control, use CloseWithTimeout() instead.
func (wm *watchManager[T]) close() {
	// We intentionally ignore the error here since this is the simple close method
	// that doesn't return errors. For error handling, use CloseWithTimeout directly.
	_ = wm.closeWithTimeout(DefaultShutdownTimeout) //nolint:errcheck
}

// CloseWithTimeout stops all watch operations and closes the manager with a timeout.
// If the timeout is exceeded, it will force-close remaining watchers and return ErrShutdownTimeout.
// This method ensures that the shutdown process doesn't hang indefinitely even if some
// watchers are unresponsive.
//
// Parameters:
//   - timeout: maximum time to wait for graceful shutdown
//
// Returns:
//   - nil if all watchers closed successfully within the timeout
//   - ErrShutdownTimeout if the timeout was exceeded
//   - ErrInvalidShutdownTimeout if the timeout value is invalid
func (wm *watchManager[T]) CloseWithTimeout(timeout time.Duration) error {
	if timeout < MinShutdownTimeout || timeout > MaxShutdownTimeout {
		return ErrInvalidShutdownTimeout
	}
	return wm.closeWithTimeout(timeout)
}

// closeWithTimeout is the internal implementation of graceful shutdown with timeout
func (wm *watchManager[T]) closeWithTimeout(timeout time.Duration) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.closed {
		return nil
	}

	wm.closed = true

	// Create a context with timeout for coordinated shutdown
	ctx, cancel := context.WithTimeout(context.Background(), timeout) //nolint:contextcheck
	defer cancel()

	if len(wm.watchers) == 0 {
		wm.logger.DebugContext(ctx, "watch manager closed (no active watchers)")
		return nil
	}

	// Create done channel to signal completion
	done := make(chan struct{})
	var shutdownError error

	// Start shutdown process in a goroutine
	go func() {
		defer close(done)

		// Stop all watchers - mark as closed first to prevent event dispatch race
		for _, instance := range wm.watchers {
			instance.closed.Store(true)
			instance.cancel()
		}

		// Wait for all watchers to complete by monitoring their done channels
		wm.logger.DebugContext(ctx, "waiting for watchers to shutdown", slog.Int("count", len(wm.watchers)))

		for id, instance := range wm.watchers {
			select {
			case <-instance.done:
				// Watcher completed successfully
				wm.logger.DebugContext(ctx, "watcher shutdown completed", slog.String("id", id))
			case <-ctx.Done():
				// Timeout reached, force close remaining watchers
				wm.logger.WarnContext(ctx, "watcher shutdown timed out, forcing close",
					slog.String("id", id),
					slog.Duration("timeout", timeout))
				shutdownError = ErrShutdownTimeout
				return
			}
		}
	}()

	// Wait for either completion or timeout
	select {
	case <-done:
		// All watchers closed successfully
		wm.watchers = make(map[string]*watchInstance[T])
		wm.logger.DebugContext(ctx, "watch manager closed successfully",
			slog.Duration("timeout", timeout))
		return shutdownError
	case <-ctx.Done():
		// Force close any remaining watchers
		wm.watchers = make(map[string]*watchInstance[T])
		wm.logger.ErrorContext(ctx, "watch manager shutdown timed out",
			slog.Duration("timeout", timeout))
		return ErrShutdownTimeout
	}
}

// generateWatcherID generates a unique ID for a watch instance
func (wm *watchManager[T]) generateWatcherID() string {
	id := wm.watcherCounter.Add(1)
	builder := corestrings.GetStringBuilder()
	defer corestrings.PutStringBuilder(builder)

	builder.WriteString("watcher-")
	builder.WriteString(wm.providerName)
	builder.WriteString("-")

	// Add a compact timestamp and counter
	now := time.Now().Unix()
	builder.WriteString(strconv.FormatInt(now, 10))
	builder.WriteString("-")
	builder.WriteString(strconv.FormatUint(id, 10))

	return builder.String()
}
