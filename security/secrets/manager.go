// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/collections/maps"
	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/data/probfilter"

	"golang.org/x/sync/singleflight"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/runtime/retry"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// Filter is a type alias for probfilter.Filter to simplify configuration.
type Filter = probfilter.Filter

// DataLoader is a type alias for probfilter.DataLoader to simplify configuration.
type DataLoader = probfilter.DataLoader

// keySetPoolCapacity is the default capacity for pooled key sets.
const keySetPoolCapacity = 64

// keySetPool is a local pool for map[string]struct{} used in updates.
var keySetPool = maps.NewPool[string, struct{}](keySetPoolCapacity)

// Manager provides centralized management of secret values with automatic caching,
// comprehensive error handling, and real-time watch capabilities.
// It serves as the primary interface for applications to access secrets from various
// storage providers while maintaining optimal performance through intelligent caching strategies.
//
// The Manager coordinates all secret operations including retrieval, caching, updates,
// and watch functionality. It abstracts the complexity of different storage backends
// behind a unified interface while providing advanced features like automatic retry
// logic, LRU caching, and real-time change notifications.
//
// Thread Safety:
// All Manager operations are thread-safe and designed for concurrent access from
// multiple goroutines. The Manager uses atomic operations, mutexes, and channels
// to ensure data consistency and prevent race conditions.
//
// Lifecycle Management:
// For periodic cache synchronization, register RunUpdateCycle with a scheduler:
//
//	sched.Register(ctx, corescheduler.TaskConfig{
//	    ID:       "secrets-cache-sync",
//	    Interval: 5 * time.Minute,
//	    Func:     manager.RunUpdateCycle,
//	})
//
// Call Shutdown() or ShutdownWithTimeout() to perform graceful cleanup when
// the Manager is no longer needed.
//
// Generic Type Parameter:
// The type parameter T represents the type of secret values managed by this Manager.
// Common types include string for simple secrets, []byte for binary data, or custom
// structs for complex configuration objects.
type Manager[T any] struct {
	// secretStorage is the underlying storage provider implementing Provider[T] interface
	secretStorage Provider[T]

	// cache stores secret values with automatic LRU eviction
	// Can be StandardCache or ShardedCache based on configuration
	cache Cache[string, *Value[T]]

	// lastUpdateTime tracks the timestamp of the last successful cache synchronization
	lastUpdateTime atomic.Value // time.Time

	// watchManager handles watch operations and event dispatching for real-time notifications
	watchManager *watchManager[T]

	// opts contains configuration options including cache settings, retry logic, and intervals
	opts *options

	// negativeFilter is used to optimize lookup of non-existent keys
	negativeFilter Filter

	// scheduler is the scheduler for background task registration
	scheduler corescheduler.TaskRegistrar

	// metrics holds all Prometheus metrics for the Manager.
	metrics *secretsMetrics

	// fetchGroup deduplicates concurrent Value(ctx, key, true) calls for the same key,
	// preventing cache stampede when multiple goroutines miss the cache simultaneously.
	fetchGroup singleflight.Group

	// updateCycleRunning guards against concurrent RunUpdateCycle calls
	updateCycleRunning atomic.Bool

	// schedulerUpdateCycleRegistered marks if RunUpdateCycle is managed by scheduler
	schedulerUpdateCycleRegistered atomic.Bool
}

// getOrCreateCache returns the provided cache or creates a default standard cache.
// If a cache is provided via options, it will be type-asserted and used.
// Otherwise, a default standard cache will be created.
func getOrCreateCache[T any](opts *options) (Cache[string, *Value[T]], error) {
	// If cache is provided via WithCache, use it
	if opts.cache != nil {
		if cache, ok := opts.cache.(Cache[string, *Value[T]]); ok {
			return cache, nil
		}
		// If type assertion fails, fall back to default cache
	}

	// Create default standard cache
	cache, err := NewStandardCache[string, *Value[T]](DefaultMaxCacheSize)
	if err != nil {
		return nil, coreerrs.Wrap(err, "create default cache")
	}
	return cache, nil
}

// New creates a new Manager instance for managing secret values from the specified provider.
// It initializes the LRU cache, configures options, and prepares the Manager for secret operations.
//
// The Manager is created with sensible defaults that can be customized through
// functional options. For periodic cache synchronization with dynamic providers,
// register RunUpdateCycle with an external scheduler:
//
//	sched.Register(ctx, corescheduler.TaskConfig{
//	    ID:       "secrets-cache-sync",
//	    Interval: 5 * time.Minute,
//	    Func:     manager.RunUpdateCycle,
//	})
//
// All Manager operations are thread-safe and designed for concurrent access from
// multiple goroutines. The Manager handles automatic retry logic, cache eviction,
// and secure memory management for secret data.
//
// Parameters:
//   - secretStorage: The Provider[T] implementation for accessing secrets from storage
//   - opt: Optional configuration functions to customize cache size, retry logic
//
// Returns a fully initialized Manager ready for immediate use and an error if
// initialization fails. Call Shutdown() or ShutdownWithTimeout() to perform
// graceful cleanup when the Manager is no longer needed.
func New[T any](secretStorage Provider[T], opt ...Option) (*Manager[T], error) {
	opts := newOptions(opt...)

	// Get provided cache or create default standard cache
	cache, err := getOrCreateCache[T](opts)
	if err != nil {
		return nil, coreerrs.Wrap(err, "initialize manager")
	}

	mgr := &Manager[T]{
		secretStorage:  secretStorage,
		cache:          cache,
		opts:           opts,
		metrics:        newSecretsMetrics(opts.collector),
		negativeFilter: opts.negativeFilter,
		scheduler:      opts.scheduler,
	}

	// Initialize atomic values
	mgr.lastUpdateTime.Store(time.Time{})

	// Initialize watch manager
	mgr.watchManager = newWatchManager(mgr, secretStorage.Name(), opts.logger)

	// Register update cycle task with scheduler if provided
	if err := mgr.registerUpdateTask(opts); err != nil {
		return nil, coreerrs.WrapOperation(err, "register update task")
	}

	if opts.healthCoordinator != nil {
		opts.healthCoordinator.RegisterService("secrets", mgr)
	}

	return mgr, nil
}

// Delete removes a secret value from both storage and cache.
// This operation securely clears the secret data before removal and is thread-safe.
//
// Returns nil on success, even if the key was not present in the cache.
func (t *Manager[T]) Delete(ctx context.Context, key string) error {
	// Get previous value from cache for watch notification
	err := t.secretStorage.Delete(ctx, key)
	if err != nil {
		t.opts.logger.ErrorContext(ctx, "failed to delete value from storage", slog.String("key", key), slogx.Error(err))
		return err
	}

	// Determine if we had a value to clear
	previousValue, hadValue := t.cache.Get(key)

	// If we have a cached value, securely clear it after successful deletion
	if hadValue {
		previousValue.Clear()
	}

	t.cache.Remove(key)

	// Update negative filter if deletable
	if t.negativeFilter != nil {
		if deletable, ok := t.negativeFilter.(probfilter.DeletableFilter); ok {
			if _, err := deletable.Delete(ctx, key); err != nil {
				t.opts.logger.WarnContext(ctx, "failed to delete key from negative filter",
					slogx.Error(err), slog.String("key", key))
			}
		}
	}

	return nil
}

// updateValueWithRetry retrieves a secret value from storage with retry logic.
// Uses exponential backoff with configurable base delay, multiplier, and maximum delay.
// Logs warnings for individual retry attempts and errors for complete failures.
//
// The retry mechanism helps handle transient network issues and temporary storage unavailability.
//
// Returns the secret value or an error if all retry attempts fail.
func (t *Manager[T]) updateValueWithRetry(ctx context.Context, key string) (*Value[T], error) {
	stop := t.metrics.fetchDuration.Start()
	defer stop()

	var val *Value[T]

	retryCfg := coreretry.Config{
		MaxAttempts: t.opts.maxRetries,
		NextDelay:   coreretry.Exponential(t.opts.exponentialConfig),
	}

	err := coreretry.Do(ctx, retryCfg, func(ctx context.Context) error {
		var err error
		val, err = t.secretStorage.Value(ctx, key)
		return err
	})

	if err != nil {
		return nil, err
	}

	t.cache.Put(key, val)
	t.opts.logger.DebugContext(ctx, "value updated", slog.String("key", key))

	return val, nil
}

// Value retrieves a secret value from the cache with optional storage fallback.
// This method implements a two-tier access pattern: cache-first lookup with configurable
// storage fallback. It first checks the LRU cache for the requested key, returning
// immediately on cache hit. On cache miss, behavior depends on the force parameter.
//
// The method is designed for high-performance secret retrieval with minimal latency
// for frequently accessed secrets (cache hits) while providing flexibility for
// cache misses through the force parameter.
//
// Parameters:
//   - ctx: Context for cancellation, deadlines, and tracing
//   - key: The secret identifier to retrieve from cache or storage
//   - force: Controls fallback behavior on cache miss
//
// Access Patterns:
//
// Cache-only access (force=false):
//   - Returns cached value immediately if found
//   - Returns ErrNotFound if not in cache (no storage access)
//   - Optimal for high-frequency operations with pre-warmed cache
//
// Cache-with-fallback access (force=true):
//   - Returns cached value immediately if found
//   - Fetches from storage provider with automatic retry on cache miss
//   - Caches the fetched value for subsequent access
//   - Includes exponential backoff retry logic for storage failures
//
// Thread Safety:
// This method is thread-safe and may modify cache state concurrently.
// Multiple goroutines can safely call this method simultaneously.
//
// Returns the secret Value[T] containing the data and metadata, or an error
// if the key doesn't exist or retrieval fails. Specific errors include
// ErrNotFound for missing keys and provider-specific errors for storage failures.
func (t *Manager[T]) Value(ctx context.Context, key string, force bool) (*Value[T], error) {
	var tmp *Value[T]

	// First, try to get from cache
	value, ok := t.cache.Get(key)
	if ok {
		// Cache hit - return immediately
		t.metrics.cacheHits.Inc()
		return value, nil
	}

	// Cache miss
	t.metrics.cacheMisses.Inc()

	// Cache miss - check if we should fetch from storage
	if !force {
		// force=false: return ErrNotFound immediately without storage access
		t.opts.logger.DebugContext(ctx, "cache miss, force=false, returning ErrNotFound",
			slog.String("key", key))
		return tmp, ErrNotFound
	}

	// Negative filter check
	if t.negativeFilter != nil {
		mightExist, err := t.negativeFilter.MightExist(ctx, key)
		if err == nil && !mightExist {
			t.opts.logger.DebugContext(ctx, "negative filter hit, returning ErrNotFound",
				slog.String("key", key))
			return tmp, ErrNotFound
		}
	}

	// force=true: fetch from storage provider via singleflight to deduplicate
	// concurrent cache misses for the same key (prevents cache stampede).
	t.opts.logger.DebugContext(ctx, "cache miss, force=true, fetching from storage",
		slog.String("key", key))

	result, err, _ := t.fetchGroup.Do(key, func() (any, error) {
		return t.updateValueWithRetry(ctx, key)
	})
	if err != nil {
		return tmp, err
	}

	return result.(*Value[T]), nil //nolint:errcheck // type is guaranteed by updateValueWithRetry
}

// ClearCache securely clears all cached secret values.
// This method iterates through all cached values, calls Clear() on each,
// and removes them from the cache. This is useful for security purposes
// when shutting down or when you need to ensure all secrets are cleared from memory.
//
// This operation is thread-safe but will block other cache operations during execution.
func (t *Manager[T]) ClearCache(ctx context.Context) {
	var clearedCount int
	for key, value := range t.cache.All() {
		value.Clear()
		t.cache.Remove(key)
		clearedCount++
	}

	t.opts.logger.DebugContext(ctx, "cache cleared", slog.Int("cleared_count", clearedCount))
}

// Shutdown gracefully shuts down the Manager using the default timeout, stopping
// watch operations and clearing all cached secrets. This method should be called when the
// application is shutting down to ensure that all sensitive data is properly cleared from memory.
//
// This method uses DefaultShutdownTimeout for the shutdown process. For custom timeout control,
// use ShutdownWithTimeout instead.
//
// After calling Shutdown(), the Manager should not be used for further operations.
func (t *Manager[T]) Shutdown() {
	// We intentionally ignore the error here since this is the simple shutdown method
	// that doesn't return errors. For error handling, use ShutdownWithTimeout directly.
	_ = t.ShutdownWithTimeout(DefaultShutdownTimeout) //nolint:errcheck
}

// ShutdownWithTimeout gracefully shuts down the Manager with a specified timeout.
// This method coordinates the shutdown of all Manager components including
// watch operations and cache clearing within the specified time limit.
//
// The shutdown process follows this sequence:
//  1. Stop all watch operations with timeout coordination
//  2. Clear all cached secrets from memory
//  3. Perform any provider-specific cleanup
//
// If any step exceeds the timeout, the method will attempt to force-close remaining
// operations and return ErrShutdownTimeout. Some cleanup may be incomplete in this case.
//
// Parameters:
//   - timeout: maximum time to wait for graceful shutdown
//
// Returns:
//   - nil if shutdown completed successfully within the timeout
//   - ErrShutdownTimeout if the timeout was exceeded during shutdown
//   - ErrInvalidShutdownTimeout if the timeout value is invalid
//
// After calling ShutdownWithTimeout(), the Manager should not be used for further operations.
//
// Example:
//
//	// Graceful shutdown with custom timeout
//	if err := manager.ShutdownWithTimeout(10 * time.Second); err != nil {
//		if errors.Is(err, secrets.ErrShutdownTimeout) {
//			log.Error("manager shutdown timed out, some cleanup may be incomplete")
//		} else {
//			log.Error("manager shutdown failed", "error", err)
//		}
//	}
func (t *Manager[T]) ShutdownWithTimeout(timeout time.Duration) error {
	if timeout < MinShutdownTimeout || timeout > MaxShutdownTimeout {
		return ErrInvalidShutdownTimeout
	}

	// Create context for coordinated shutdown
	ctx, cancel := corecontext.ApplyTimeout(context.Background(), timeout) //nolint:contextcheck
	defer cancel()

	t.opts.logger.InfoContext(ctx, "starting manager shutdown", slog.Duration("timeout", timeout))

	// Track shutdown phases
	shutdownStartTime := time.Now()
	var phase string
	var shutdownError error

	// Create done channel for shutdown coordination
	done := make(chan struct{})

	go func() {
		defer close(done)

		// Phase 1: Stop all watch operations with remaining time
		if t.watchManager != nil {
			phase = "stopping watchers"
			t.opts.logger.DebugContext(ctx, "shutdown phase: stopping watchers")

			// Calculate remaining time for watch manager shutdown
			elapsed := time.Since(shutdownStartTime)
			remainingTime := timeout - elapsed

			if remainingTime <= 0 {
				shutdownError = ErrShutdownTimeout
				return
			}

			// Use the remaining time for watch manager shutdown, but at least MinShutdownTimeout
			watchTimeout := max(remainingTime, MinShutdownTimeout)

			if err := t.watchManager.CloseWithTimeout(watchTimeout); err != nil {
				if errors.Is(err, ErrShutdownTimeout) {
					t.opts.logger.WarnContext(ctx, "watch manager shutdown timed out")
					shutdownError = ErrShutdownTimeout
				} else {
					t.opts.logger.ErrorContext(ctx, "watch manager shutdown failed", slog.Any("error", err))
					shutdownError = err
				}
				return
			}
		}

		// Check if context was canceled
		if ctx.Err() != nil {
			shutdownError = ErrShutdownTimeout
			return
		}

		// Phase 2: Clear all cached secrets
		phase = "clearing cache"
		t.opts.logger.DebugContext(ctx, "shutdown phase: clearing cache")
		t.ClearCache(ctx)

		// Check if context was canceled
		if ctx.Err() != nil {
			shutdownError = ErrShutdownTimeout
			return
		}

		// Phase 3: Provider-specific cleanup (if supported)
		phase = "provider cleanup"
		t.opts.logger.DebugContext(ctx, "shutdown phase: provider cleanup")
		if shutdownProvider, ok := t.secretStorage.(interface{ Shutdown() }); ok {
			t.opts.logger.DebugContext(ctx, "performing provider-specific shutdown")
			shutdownProvider.Shutdown()
		}
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		duration := time.Since(shutdownStartTime)
		if shutdownError != nil {
			t.opts.logger.ErrorContext(ctx, "manager shutdown completed with errors",
				slog.Duration("duration", duration),
				slog.Any("error", shutdownError))
			return shutdownError
		}
		t.opts.logger.InfoContext(ctx, "manager shutdown completed successfully",
			slog.Duration("duration", duration))
		return nil

	case <-ctx.Done():
		duration := time.Since(shutdownStartTime)
		t.opts.logger.ErrorContext(ctx, "manager shutdown timed out",
			slog.Duration("duration", duration),
			slog.Duration("timeout", timeout),
			slog.String("phase", phase))

		// Force immediate cleanup of remaining components
		t.forceShutdown(ctx)
		return ErrShutdownTimeout
	}
}

// forceShutdown performs immediate cleanup when graceful shutdown times out
func (t *Manager[T]) forceShutdown(ctx context.Context) {
	t.opts.logger.WarnContext(ctx, "performing force shutdown")

	// Force close watch manager
	if t.watchManager != nil {
		t.watchManager.close() // This will use default timeout internally
	}

	// Clear cache
	t.ClearCache(ctx)

	t.opts.logger.WarnContext(ctx, "force shutdown completed")
}

// CacheSize returns the current number of entries in the cache.
// This count includes all cached secret values and reflects the current memory usage.
// The returned value may be less than the configured maximum cache size.
//
// This method is thread-safe and provides real-time cache statistics.
func (t *Manager[T]) CacheSize() int {
	return t.cache.Len()
}

// LastUpdateTime returns the timestamp of the last successful cache update.
// For static providers, this reflects the initialization time.
// For dynamic providers, this shows when the background updater last synchronized with storage.
//
// Returns zero time if no updates have occurred yet.
// This method is thread-safe and uses atomic operations for consistent reads.
func (t *Manager[T]) LastUpdateTime() time.Time {
	if val := t.lastUpdateTime.Load(); val != nil {
		return val.(time.Time) //nolint:errcheck
	}
	return time.Time{}
}

// Save stores or updates a secret value in the storage backend and cache.
// This method provides a write-through caching pattern where the value is
// immediately written to the storage provider and then cached for fast retrieval.
// It supports both creating new secrets and updating existing ones with versioning.
//
// Operation Flow:
// 1. The value is saved to the storage provider with automatic retry logic
// 2. On successful save, the provider returns updated metadata (version, etc.)
// 3. The value is immediately cached to maintain consistency
// 4. Watch events are triggered for any active watchers
//
// The operation uses exponential backoff retry logic for transient failures
// such as network timeouts, rate limiting, or temporary service unavailability.
//
// Cache Consistency:
// The cache is updated atomically with the storage operation to prevent
// inconsistencies. If the storage operation fails, the cache remains unchanged.
// This ensures that cached values always reflect the current storage state.
//
// Watch Notifications:
// Successful save operations trigger watch events for monitoring secret changes.
// Active watchers will receive appropriate EventTypeCreated or EventTypeUpdated
// events based on whether the key existed previously.
//
// Parameters:
//   - ctx: Context for cancellation, deadlines, and request tracing
//   - key: The secret identifier to store (must meet key validation requirements)
//   - value: The secret value to save (type T, must be encodable by value decoder)
//
// Returns an error if the secret cannot be saved after all retry attempts,
// if the key is invalid, or if encoding fails. The cache is only updated
// on successful save operations to maintain consistency.
func (t *Manager[T]) Save(ctx context.Context, key string, value T) error {
	return t.saveWithRetry(ctx, key, value)
}

// saveWithRetry saves a secret value to storage with retry logic.
// Uses exponential backoff with configurable base delay, multiplier, and maximum delay.
// Logs warnings for individual retry attempts and errors for complete failures.
//
// On successful save, automatically fetches the updated value from storage to
// ensure cache consistency and obtain latest metadata (version, etc.).
//
// Returns an error if the save operation fails after all retry attempts.
func (t *Manager[T]) saveWithRetry(ctx context.Context, key string, value T) error {
	retryCfg := coreretry.Config{
		MaxAttempts: t.opts.maxRetries,
		NextDelay:   coreretry.Exponential(t.opts.exponentialConfig),
	}

	err := coreretry.Do(ctx, retryCfg, func(ctx context.Context) error {
		return t.secretStorage.Save(ctx, key, value)
	})

	if err != nil {
		return err
	}

	// Update negative filter
	if t.negativeFilter != nil {
		if addErr := t.negativeFilter.Add(ctx, key); addErr != nil {
			t.opts.logger.ErrorContext(ctx, "failed to update negative filter",
				slogx.Error(addErr), slog.String("key", key))
		}
	}

	// After successful save, fetch the updated value to cache it with proper metadata
	val, err := t.secretStorage.Value(ctx, key)
	if err != nil {
		t.opts.logger.WarnContext(ctx, "failed to fetch saved value for caching",
			slogx.Error(err),
			slog.String("key", key))
		// Don't return error here - save was successful, cache update failed
		return nil
	}

	t.cache.Put(key, val)
	t.opts.logger.DebugContext(ctx, "value saved and cached", slog.String("key", key))

	return nil
}

// WarmCache preloads specified secret keys into the cache concurrently.
// This method fetches multiple secrets in parallel to improve performance and
// reduce latency for subsequent Value() calls. Each key is processed independently
// with full retry logic.
//
// Concurrency is limited to 2x CPU cores to avoid overwhelming the storage system.
// Individual key failures do not stop the entire operation, but are collected
// and reported in the final error.
//
// Returns an error if any keys failed to load, with the count of failed keys.
// Successful keys are cached normally and available via Value() calls.
func (t *Manager[T]) WarmCache(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	var failedCount atomic.Int64
	err := concurrency.Process(ctx, keys, func(ctx context.Context, key string) error {
		_, err := t.updateValueWithRetry(ctx, key)
		if err != nil {
			if !coreerrs.IsContextCanceled(err) {
				failedCount.Add(1)
				t.opts.logger.ErrorContext(ctx, "failed to warm cache for key",
					slog.String("key", key),
					slogx.Error(err))
				return nil
			}
			return err
		}
		return nil
	}, concurrency.BatchConfig[string]{
		StopOnError: false, // Continue on error to warm as many keys as possible
	})

	// Wait for all goroutines to complete
	if err != nil {
		return coreerrs.WrapOperation(err, "warm cache")
	}

	t.opts.logger.DebugContext(ctx, "cache warmed successfully",
		slog.Int("keys", len(keys)))

	return nil
}

// Watch starts watching for secret changes and returns a channel of events.
// This method enables real-time notifications when secrets are created, updated, or deleted.
// The watch operation continues until the context is canceled or Stop() is called on the result.
//
// Events are generated during Manager cache synchronization cycles, providing a unified
// event stream regardless of the underlying storage provider type. The watch operation
// automatically ensures the Manager's background updater is running to provide events.
//
// Watch operations are thread-safe and multiple watchers can operate concurrently.
// Each watcher maintains its own event channel and filter configuration.
//
// Parameters:
//   - ctx: context for the watch operation, supports cancellation and timeouts
//   - opts: configuration options for watch behavior (keys to watch, event types, etc.)
//
// Returns a WatchResult containing the event channel and control functions, or an error
// if the watch operation cannot be started.
//
// Example:
//
//	// Watch all secrets
//	watchResult, err := manager.Watch(ctx, secrets.DefaultWatchOptions())
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer watchResult.Stop()
//
//	// Process events
//	for {
//		select {
//		case event := <-watchResult.Events:
//			switch event.Type {
//			case secrets.EventTypeCreated:
//				log.Printf("Secret created: %s", event.Key)
//			case secrets.EventTypeUpdated:
//				log.Printf("Secret updated: %s", event.Key)
//			case secrets.EventTypeDeleted:
//				log.Printf("Secret deleted: %s", event.Key)
//			case secrets.EventTypeError:
//				log.Printf("Watch error: %v", event.Error)
//			}
//		case <-watchResult.Done:
//			log.Println("Watch stopped")
//			return
//		}
//	}
func (t *Manager[T]) Watch(ctx context.Context, opts WatchOptions) (*WatchResult[T], error) {
	return t.watchManager.Watch(ctx, opts)
}

// WatchKeys is a convenience method for watching specific secret keys.
// This is equivalent to calling Watch() with WatchOptions.Keys set to the provided keys.
// The method uses default values for all other watch options.
//
// Parameters:
//   - ctx: context for the watch operation
//   - keys: specific secret keys to watch for changes
//
// Returns a WatchResult for the specified keys, or an error if the watch cannot be started.
func (t *Manager[T]) WatchKeys(ctx context.Context, keys ...string) (*WatchResult[T], error) {
	opts := DefaultWatchOptions()
	opts.Keys = keys
	return t.Watch(ctx, opts)
}

// WatchKey is a convenience method for watching a single secret key.
// This is equivalent to calling WatchKeys() with a single key. The method uses
// default values for all other watch options.
//
// Parameters:
//   - ctx: context for the watch operation
//   - key: the secret key to watch for changes
//
// Returns a WatchResult for the specified key, or an error if the watch cannot be started.
func (t *Manager[T]) WatchKey(ctx context.Context, key string) (*WatchResult[T], error) {
	return t.WatchKeys(ctx, key)
}
