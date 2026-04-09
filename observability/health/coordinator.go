// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"iter"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"

	corectx "github.com/altessa-s/go-atlas/core/context"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Subscriber represents an active subscription to health status updates.
// Implementations must be safe for use from a single goroutine; the caller
// must not share a Subscriber across goroutines without synchronization.
// Call [Subscriber.Close] when done to release resources.
type Subscriber interface {
	// Updates returns a read-only channel that receives [ServingStatus] changes
	// detected by [Coordinator.RunHealthCheckCycle].
	Updates() <-chan ServingStatus
	// InitialStatus returns the [ServingStatus] captured at subscription time.
	InitialStatus() ServingStatus
	// Close terminates the subscription and releases resources.
	// It is safe to call Close multiple times.
	Close()
}

// Checker defines the interface for health check implementations.
// Register implementations via [Coordinator.RegisterService].
// See [Func] for a convenient function adapter.
type Checker interface {
	// CheckHealth returns the current [ServingStatus].
	// The context carries the per-check timeout configured via [WithCheckTimeout].
	CheckHealth(ctx context.Context) ServingStatus
}

// Func is a function adapter implementing the [Checker] interface.
//
// Example:
//
//	coordinator.RegisterService("api", health.Func(func(ctx context.Context) health.ServingStatus {
//	    if apiClient.Ping(ctx) != nil {
//	        return health.StatusNotServing
//	    }
//	    return health.StatusServing
//	}))
type Func func(context.Context) ServingStatus

// CheckHealth calls the underlying function and returns its result.
func (f Func) CheckHealth(ctx context.Context) ServingStatus {
	return f(ctx)
}

// Notifier exposes health status notifications for domain components.
// [Coordinator] implements this interface.
type Notifier interface {
	// NotifyStatusChange sends a [ServingStatus] update to all watchers of a service.
	NotifyStatusChange(service string, status ServingStatus)
	// TriggerRecheckAll clears all cached statuses, forcing a fresh check on the next poll cycle.
	TriggerRecheckAll()
	// BroadcastStatus sends the same [ServingStatus] to all watchers regardless of service.
	BroadcastStatus(status ServingStatus)
	// Close gracefully shuts down the notifier.
	Close()
}

// HealthCoordinator defines the core health checking operations.
// [Coordinator] is the concrete implementation.
type HealthCoordinator interface {
	// Subscribe creates a [Subscriber] for a service's health updates.
	// Returns [ErrWatcherLimitExceeded] or [ErrCoordinatorShutdown] on failure.
	Subscribe(ctx context.Context, service string) (Subscriber, error)
	// ListStatuses returns all services and their current [ServingStatus] values.
	ListStatuses(ctx context.Context) (map[string]ServingStatus, error)
	// CheckStatus returns the current [ServingStatus] for a service.
	// Pass empty string to check overall health.
	CheckStatus(ctx context.Context, service string) ServingStatus
	// Close gracefully shuts down the coordinator and all active subscriptions.
	Close()
}

// Metrics exposes runtime performance counters for the [Coordinator].
type Metrics interface {
	// GetMetrics returns a snapshot of current runtime metrics including
	// "active_watchers", "list_calls_in_flight", "cached_statuses", and "total_watchers".
	GetMetrics() map[string]any
}

// Coordinator manages health status, caching, and watcher subscriptions.
// It is transport-agnostic and safe for concurrent use.
//
// For periodic health checks, use WithScheduler and WithCheckSchedule options:
//
//	coordinator := health.New(
//	    health.WithScheduler(sched),
//	    health.WithCheckSchedule("*/5 * * * * *"),
//	    health.WithLogger(logger),
//	)
//
// Or register RunHealthCheckCycle manually with service/scheduler:
//
//	sched.Register(ctx, corescheduler.TaskConfig{
//	    ID:         "health-check",
//	    Schedule:   "*/5 * * * * *", // every 5 seconds
//	    Func:       coordinator.RunHealthCheckCycle,
//	    RunOnStart: true,
//	})
type Coordinator struct {
	servicesMu sync.RWMutex
	services   map[string]Checker

	watcherShards []watcherShard

	watcherChannelBuffer      int
	maxWatchersPerService     int
	numShards                 int
	maxConcurrentHealthChecks int
	statusCacheTTL            time.Duration
	checkTimeout              time.Duration
	adaptiveBufferThreshold   int32
	adaptiveBufferMultiplier  int
	maxAdaptiveBuffer         int

	healthCheckSem chan struct{}

	statusCache sync.Map // map[string]*cachedStatus

	logger    *slog.Logger
	scheduler corescheduler.TaskRegistrar

	metrics *healthMetrics

	activeWatchers    atomic.Int32
	listCallsInFlight atomic.Int32

	closed             atomic.Bool
	healthCheckRunning atomic.Bool // Guards against concurrent RunHealthCheckCycle calls.

	schedulerHealthCheckRegistered atomic.Bool // Marks if RunHealthCheckCycle is managed by scheduler.
}

// Compile-time interface assertions.
var (
	_ Notifier = (*Coordinator)(nil)
	_ Metrics  = (*Coordinator)(nil)
)

// New creates a new Coordinator with the given options.
//
// For periodic health checks, use WithScheduler and WithCheckSchedule options:
//
//	coordinator := health.New(
//	    health.WithScheduler(sched),
//	    health.WithCheckSchedule("*/5 * * * * *"),
//	    health.WithLogger(logger),
//	)
func New(opts ...Option) *Coordinator {
	o := newOptions(opts...)
	c := &Coordinator{
		metrics:                   newHealthMetrics(o.collector),
		services:                  make(map[string]Checker),
		watcherChannelBuffer:      o.watcherChannelBuffer,
		maxWatchersPerService:     o.maxWatchersPerService,
		numShards:                 o.numShards,
		maxConcurrentHealthChecks: o.maxConcurrentHealthChecks,
		statusCacheTTL:            o.statusCacheTTL,
		checkTimeout:              o.checkTimeout,
		adaptiveBufferThreshold:   o.adaptiveBufferThreshold,
		adaptiveBufferMultiplier:  o.adaptiveBufferMultiplier,
		maxAdaptiveBuffer:         o.maxAdaptiveBuffer,
		logger:                    o.logger,
		scheduler:                 o.scheduler,
	}

	c.watcherShards = make([]watcherShard, c.numShards)
	for i := range c.watcherShards {
		c.watcherShards[i].watchers = make(map[string]map[*watcher]struct{})
	}

	c.healthCheckSem = make(chan struct{}, c.maxConcurrentHealthChecks)

	// Register health check task with scheduler if configured
	if err := c.registerSchedulerTask(o); err != nil {
		c.logger.Warn("failed to register health check task", slog.Any("error", err))
	}

	return c
}

// Close gracefully shuts down the coordinator and all active subscriptions.
// After Close, RunHealthCheckCycle becomes a no-op.
func (c *Coordinator) Close() {
	if !c.closed.CompareAndSwap(false, true) {
		return // Already closed
	}

	// Notify all watchers of shutdown
	c.BroadcastStatus(StatusNotServing)
}

// RegisterService adds a service to the coordinator.
//
// Example:
//
//	coordinator.RegisterService("database", &DatabaseChecker{db: db})
func (c *Coordinator) RegisterService(name string, checker Checker) {
	c.servicesMu.Lock()
	defer c.servicesMu.Unlock()

	c.services[name] = checker
}

// UnregisterService removes a service and clears its cached status.
func (c *Coordinator) UnregisterService(name string) {
	c.servicesMu.Lock()
	defer c.servicesMu.Unlock()

	delete(c.services, name)
	c.statusCache.Delete(name)
}

// ListServices returns an iterator over all registered service names.
func (c *Coordinator) ListServices() iter.Seq[string] {
	return func(yield func(string) bool) {
		c.servicesMu.RLock()
		defer c.servicesMu.RUnlock()

		for name := range c.services {
			if !yield(name) {
				return
			}
		}
	}
}

// CheckServiceHealth checks a specific service's health.
// Returns StatusServiceUnknown if the service is not registered.
func (c *Coordinator) CheckServiceHealth(ctx context.Context, service string) ServingStatus {
	c.servicesMu.RLock()
	checker, ok := c.services[service]
	c.servicesMu.RUnlock()

	if !ok {
		return StatusServiceUnknown
	}

	return checker.CheckHealth(ctx)
}

// CheckHealth checks overall health by polling all registered services.
// Returns StatusServing only if all services are healthy or none are registered.
func (c *Coordinator) CheckHealth(ctx context.Context) ServingStatus {
	c.servicesMu.RLock()
	services := make(map[string]Checker, len(c.services))
	maps.Copy(services, c.services)
	c.servicesMu.RUnlock()

	if len(services) == 0 {
		return StatusServing
	}

	for _, checker := range services {
		status := checker.CheckHealth(ctx)
		if status != StatusServing {
			return StatusNotServing
		}
	}

	return StatusServing
}

func (c *Coordinator) getShardIndex(service string) int {
	// Inline FNV-1a 32-bit to avoid allocating a hash.Hash32 per call.
	const (
		offset32 uint32 = 2166136261
		prime32  uint32 = 16777619
	)
	h := offset32
	for i := range len(service) {
		h ^= uint32(service[i])
		h *= prime32
	}
	return int(h % uint32(c.numShards)) // #nosec G115 -- numShards is small positive int
}

func (c *Coordinator) getCachedStatus(service string) (ServingStatus, bool) {
	if cached, ok := c.statusCache.Load(service); ok {
		if cs, ok := cached.(*cachedStatus); ok {
			timestamp := cs.timestamp.Load()
			if time.Since(time.Unix(0, timestamp)) < c.statusCacheTTL {
				return cs.status, true
			}
		}
	}
	return StatusUnknown, false
}

func (c *Coordinator) setCachedStatus(service string, status ServingStatus) {
	cs := &cachedStatus{status: status}
	cs.timestamp.Store(time.Now().UnixNano())
	c.statusCache.Store(service, cs)
}

func (c *Coordinator) getHealthStatus(ctx context.Context, service string) ServingStatus {
	if status, ok := c.getCachedStatus(service); ok {
		return status
	}

	checkCtx, cancel := corectx.ApplyTimeout(ctx, c.checkTimeout)
	defer cancel()

	var status ServingStatus
	if service == "" {
		status = c.CheckHealth(checkCtx)
	} else {
		status = c.CheckServiceHealth(checkCtx, service)
	}
	c.setCachedStatus(service, status)
	return status
}

// CheckStatus returns a service's status using cache when available.
// Pass empty string to check overall health.
//
// Example:
//
//	status := coordinator.CheckStatus(ctx, "database")
func (c *Coordinator) CheckStatus(ctx context.Context, service string) ServingStatus {
	return c.getHealthStatus(ctx, service)
}

// ListStatuses returns all services and their current statuses.
// Concurrent checks are limited by MaxConcurrentHealthChecks.
func (c *Coordinator) ListStatuses(ctx context.Context) (map[string]ServingStatus, error) {
	c.listCallsInFlight.Add(1)
	defer c.listCallsInFlight.Add(-1)

	services := slices.Collect(c.ListServices())
	if len(services) == 0 {
		return make(map[string]ServingStatus), nil
	}

	type result struct {
		service string
		status  ServingStatus
	}

	resultsSlice, err := concurrency.ProcessCollect[string, result](ctx, services, func(ctx context.Context, svc string) (result, error) {
		checkCtx, cancel := corectx.ApplyTimeout(ctx, c.checkTimeout)
		status := c.CheckServiceHealth(checkCtx, svc)
		cancel()

		return result{service: svc, status: status}, nil
	},
		// No StopOnError: continue checking other services if one fails.
		concurrency.WithConcurrency[string](c.maxConcurrentHealthChecks),
	)

	if err != nil {
		return nil, err
	}

	results := make(map[string]ServingStatus, len(resultsSlice))
	for _, r := range resultsSlice {
		results[r.service] = r.status
	}

	return results, nil
}

// Subscribe creates a [Subscription] for a service's health updates.
// Status updates are delivered when [Coordinator.RunHealthCheckCycle] detects changes.
// Returns [ErrWatcherLimitExceeded] if the per-service limit is reached,
// or [ErrCoordinatorShutdown] if the coordinator has been closed.
//
// Example:
//
//	sub, err := coordinator.Subscribe(ctx, "database")
//	if err != nil {
//	    return err
//	}
//	defer sub.Close()
func (c *Coordinator) Subscribe(ctx context.Context, service string) (Subscriber, error) {
	if c.closed.Load() {
		return nil, ErrCoordinatorShutdown
	}

	initialStatus := c.getHealthStatus(ctx, service)

	bufferSize := c.watcherChannelBuffer
	if active := c.activeWatchers.Load(); active > c.adaptiveBufferThreshold {
		bufferSize = min(bufferSize*c.adaptiveBufferMultiplier, c.maxAdaptiveBuffer)
	}

	w := newWatcher(bufferSize, initialStatus)
	shardIdx := c.getShardIndex(service)
	shard := &c.watcherShards[shardIdx]

	shard.mu.Lock()
	if shard.watchers[service] == nil {
		shard.watchers[service] = make(map[*watcher]struct{})
	}
	if c.maxWatchersPerService > 0 && len(shard.watchers[service]) >= c.maxWatchersPerService {
		shard.mu.Unlock()
		w.close()
		return nil, ErrWatcherLimitExceeded
	}
	shard.watchers[service][w] = struct{}{}
	shard.mu.Unlock()

	c.activeWatchers.Add(1)

	c.logger.Debug("new subscription created",
		slog.String("service", service),
		slog.Int("buffer_size", bufferSize),
		slog.String("initial_status", initialStatus.String()))

	return &Subscription{
		initialStatus: initialStatus,
		updates:       w.ch,
		cancel: func() {
			shard := &c.watcherShards[shardIdx]
			shard.mu.Lock()
			delete(shard.watchers[service], w)
			if len(shard.watchers[service]) == 0 {
				delete(shard.watchers, service)
			}
			shard.mu.Unlock()

			w.close()
			c.activeWatchers.Add(-1)
		},
	}, nil
}

// NotifyStatusChange sends a status update to all watchers of a service.
// The status is cached and delivered to active subscriptions.
func (c *Coordinator) NotifyStatusChange(service string, status ServingStatus) {
	c.setCachedStatus(service, status)

	shardIdx := c.getShardIndex(service)
	shard := &c.watcherShards[shardIdx]

	shard.mu.RLock()
	watchers := shard.watchers[service]
	if watchers == nil {
		shard.mu.RUnlock()
		return
	}

	list := make([]*watcher, 0, len(watchers))
	for w := range watchers {
		list = append(list, w)
	}
	shard.mu.RUnlock()

	for _, w := range list {
		w.notify(status)
	}

	c.logger.Debug("status change notified",
		slog.String("service", service),
		slog.String("status", status.String()),
		slog.Int("watchers", len(list)))
}

// TriggerRecheckAll invalidates all cached statuses.
// Watchers fetch fresh status on their next poll cycle.
func (c *Coordinator) TriggerRecheckAll() {
	c.statusCache.Range(func(key, value any) bool {
		c.statusCache.Delete(key)
		return true
	})
}

// BroadcastStatus sends the same status to all watchers across all services.
// Useful for shutdown scenarios.
func (c *Coordinator) BroadcastStatus(status ServingStatus) {
	for i := range c.watcherShards {
		shard := &c.watcherShards[i]

		shard.mu.RLock()
		var list []*watcher
		for _, serviceWatchers := range shard.watchers {
			for w := range serviceWatchers {
				list = append(list, w)
			}
		}
		shard.mu.RUnlock()

		for _, w := range list {
			w.notify(status)
		}
	}
}

// GetMetrics returns runtime metrics for monitoring.
//
// Example:
//
//	metrics := coordinator.GetMetrics()
//	fmt.Println("Active watchers:", metrics["active_watchers"])
func (c *Coordinator) GetMetrics() map[string]any {
	metrics := make(map[string]any)

	metrics["active_watchers"] = c.activeWatchers.Load()
	metrics["list_calls_in_flight"] = c.listCallsInFlight.Load()

	cached := 0
	c.statusCache.Range(func(key, value any) bool {
		cached++
		return true
	})
	metrics["cached_statuses"] = cached

	totalWatchers := 0
	for i := range c.watcherShards {
		shard := &c.watcherShards[i]
		shard.mu.RLock()
		for _, watchers := range shard.watchers {
			totalWatchers += len(watchers)
		}
		shard.mu.RUnlock()
	}
	metrics["total_watchers"] = totalWatchers

	return metrics
}
