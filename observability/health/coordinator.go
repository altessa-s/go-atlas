// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"fmt"
	"iter"
	"slices"

	"github.com/altessa-s/go-atlas-core/runtime/singleflight"

	corectx "github.com/altessa-s/go-atlas-core/context"
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

// Registrar is the registration-only port for components that add a
// [Checker] to the health system once at construction. [Coordinator]
// implements this interface.
type Registrar interface {
	// RegisterService adds a service checker under name.
	RegisterService(name string, checker Checker)
}

// Reporter is the port for components that register a [Checker] and push
// [ServingStatus] changes as they observe them. [Coordinator] implements
// this interface.
type Reporter interface {
	Registrar
	// NotifyStatusChange sends a [ServingStatus] update to all watchers of a service.
	NotifyStatusChange(service string, status ServingStatus)
}

// ScopedReporter registers a checker for the lifetime of its owner. Both
// callbacks become no-ops once another registration replaces this one.
type ScopedReporter interface {
	RegisterScopedService(name string, checker Checker) (notify func(ServingStatus), unregister func())
}

// DynamicReporter is the port for components whose set of registered
// services changes at runtime: they register, report, and unregister
// services as the underlying targets come and go. [Coordinator]
// implements this interface.
type DynamicReporter interface {
	Reporter
	// UnregisterService removes a service and clears its cached status.
	UnregisterService(name string)
}

var (
	_ Registrar       = (*Coordinator)(nil)
	_ Reporter        = (*Coordinator)(nil)
	_ ScopedReporter  = (*Coordinator)(nil)
	_ DynamicReporter = (*Coordinator)(nil)
)

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
//	if err := coordinator.RegisterHealthChecks(ctx); err != nil {
//	    return err
//	}
//
// Or register the health check callback directly with go-horai:
//
//	sched.Register(ctx, schedtask.Config{
//	    ID:         "health-check",
//	    Schedule:   "*/5 * * * * *", // every 5 seconds
//	    Func:       coordinator.RegisterHealthCheckSchedulerFunc(),
//	    RunOnStart: true,
//	})
type Coordinator struct {
	statuses statusReporter
	watchers subscriptions
	checks   checkScheduler
}

// Compile-time interface assertions.
var (
	_ Notifier = (*Coordinator)(nil)
	_ Metrics  = (*Coordinator)(nil)
)

// New creates a new Coordinator with the given options. It does not register
// scheduled work; call RegisterHealthChecks and handle its error explicitly.
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
		statuses: statusReporter{
			registry:                  serviceRegistry{services: make(map[string]*serviceRegistration)},
			statusCacheTTL:            o.statusCacheTTL,
			checkTimeout:              o.checkTimeout,
			maxConcurrentHealthChecks: o.maxConcurrentHealthChecks,
			refresh:                   singleflight.New[string, ServingStatus](singleflight.WithTimeout(o.checkTimeout)),
		},
		watchers: newSubscriptions(o),
	}
	c.checks = checkScheduler{
		scheduler:     o.scheduler,
		checkSchedule: o.checkSchedule,
		logger:        o.logger,
		metrics:       newHealthMetrics(o.meter),
		statuses:      &c.statuses,
		watchers:      &c.watchers,
	}
	return c
}

// Close gracefully shuts down the coordinator and all active subscriptions.
// After Close, RunHealthCheckCycle becomes a no-op.
//
// Close pins the overall status to [StatusNotServing] (see
// [Coordinator.SetOverallStatus]) and notifies every active watcher with the
// same status, so both pull-based readers (e.g. HTTP /readyz handlers backed
// by [Coordinator.CheckStatus]) and push-based subscribers immediately see
// the service as not serving.
func (c *Coordinator) Close() {
	if !c.checks.closed.CompareAndSwap(false, true) {
		return // Already closed
	}

	// Make pull-API readers see NotServing immediately; pre-shutdown checkers
	// may still return Serving while connections are torn down.
	c.SetOverallStatus(StatusNotServing)

	// Notify all watchers of shutdown.
	c.BroadcastStatus(StatusNotServing)
}

// SetOverallStatus pins the result of [Coordinator.CheckHealth],
// [Coordinator.CheckServiceHealth] and [Coordinator.CheckStatus] regardless
// of registered [Checker]s. Pass [StatusUnknown] to clear the override and
// resume polling Checkers.
//
// This is the pull-API counterpart to [Coordinator.BroadcastStatus]: while
// BroadcastStatus only reaches active subscribers (e.g. gRPC Health Watch
// streams), SetOverallStatus also affects HTTP probes and any caller that
// invokes CheckHealth/CheckStatus directly. Typical use is graceful
// shutdown: call SetOverallStatus(StatusNotServing) before tearing down
// dependencies so readiness probes flip to 503 immediately.
//
// Example:
//
//	// On SIGTERM, before stopping dependencies:
//	coordinator.SetOverallStatus(health.StatusNotServing)
//	coordinator.BroadcastStatus(health.StatusNotServing)
//
// Setting the override clears the cached overall status so the next
// CheckStatus("") observes the override without waiting for cache TTL.
func (c *Coordinator) SetOverallStatus(status ServingStatus) {
	c.statuses.override.Store(int32(status))
	// Drop the cached overall status so CheckStatus("") does not return a
	// stale Serving entry from before the override was set.
	c.statuses.registry.cache.Delete("")
}

// OverallStatusOverride returns the current override set via
// [Coordinator.SetOverallStatus], or [StatusUnknown] if no override is in
// effect.
func (c *Coordinator) OverallStatusOverride() ServingStatus {
	return ServingStatus(c.statuses.override.Load())
}

// RegisterService adds a service to the coordinator.
//
// Example:
//
//	coordinator.RegisterService("database", &DatabaseChecker{db: db})
func (c *Coordinator) RegisterService(name string, checker Checker) {
	c.register(name, checker)
}

// RegisterScopedService implements [ScopedReporter]. Closing an old registration
// cannot unregister its replacement or overwrite its last reported status.
func (c *Coordinator) RegisterScopedService(name string, checker Checker) (notify func(ServingStatus), unregister func()) {
	reg := c.register(name, checker)
	return func(status ServingStatus) { c.statuses.registry.report(name, reg, status, &c.watchers) },
		func() { c.statuses.registry.remove(name, reg) }
}

func (c *Coordinator) register(name string, checker Checker) *serviceRegistration {
	reg := c.statuses.registry.register(name, checker)
	if detachable, ok := checker.(*DetachableChecker); ok {
		detachable.bind(serviceBinding{registry: &c.statuses.registry, name: name, owner: reg})
	}
	return reg
}

// UnregisterService removes a service and clears its cached status.
func (c *Coordinator) UnregisterService(name string) {
	c.statuses.registry.remove(name, nil)
}

// ListServices returns an iterator over all registered service names.
func (c *Coordinator) ListServices() iter.Seq[string] {
	return func(yield func(string) bool) {
		// Take a fresh snapshot for each iteration and yield outside the lock.
		slices.Values(c.statuses.registry.names())(yield)
	}
}

// CheckServiceHealth checks a specific service's health.
// Returns StatusServiceUnknown if the service is not registered.
//
// When an overall status override is in effect (see
// [Coordinator.SetOverallStatus]), the override is returned for any
// registered service. Unregistered services still return
// [StatusServiceUnknown] so callers can distinguish missing services from
// the shutdown signal.
func (c *Coordinator) CheckServiceHealth(ctx context.Context, service string) ServingStatus {
	return c.statuses.serviceHealth(ctx, service)
}

// Health reports the overall status as an error: nil while [CheckHealth]
// returns [StatusServing], otherwise an error wrapping [ErrNotServing] that
// names the status. It lets a Coordinator stand in wherever a
// Health(ctx) error dependency is expected — the gRPC health interceptor
// (transport/grpc/interceptors/health) among them — without an adapter.
func (c *Coordinator) Health(ctx context.Context) error {
	ctx, cancel := corectx.ApplyTimeout(ctx, c.statuses.checkTimeout)
	defer cancel()

	if st := c.CheckHealth(ctx); st != StatusServing {
		return fmt.Errorf("%w: %s", ErrNotServing, st)
	}
	return nil
}

// CheckHealth checks overall health by polling all registered services.
// Returns StatusServing only if all services are healthy or none are registered.
// Services are checked concurrently, each under the check timeout (see
// [WithCheckTimeout]), so one slow checker does not starve the others.
//
// When an overall status override is in effect (see
// [Coordinator.SetOverallStatus]), the override is returned without
// consulting any [Checker]. This lets graceful shutdown flip readiness
// before individual dependencies report as failed.
func (c *Coordinator) CheckHealth(ctx context.Context) ServingStatus {
	return c.statuses.overall(ctx)
}

// CheckStatus returns a service's status using cache when available.
// Pass empty string to check overall health.
//
// Example:
//
//	status := coordinator.CheckStatus(ctx, "database")
func (c *Coordinator) CheckStatus(ctx context.Context, service string) ServingStatus {
	return c.statuses.getHealthStatus(ctx, service)
}

// ListStatuses returns all services and their current statuses.
// Concurrent checks are limited by MaxConcurrentHealthChecks.
func (c *Coordinator) ListStatuses(ctx context.Context) (map[string]ServingStatus, error) {
	return c.statuses.list(ctx)
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
	if c.checks.closed.Load() {
		return nil, ErrCoordinatorShutdown
	}

	initialStatus := c.statuses.getHealthStatus(ctx, service)
	return c.watchers.subscribe(service, initialStatus)
}

// NotifyStatusChange sends a status update to all watchers of a service.
// The status is cached and delivered to active subscriptions.
func (c *Coordinator) NotifyStatusChange(service string, status ServingStatus) {
	c.statuses.registry.report(service, nil, status, &c.watchers)
}

// TriggerRecheckAll invalidates all cached statuses.
// Watchers fetch fresh status on their next poll cycle.
func (c *Coordinator) TriggerRecheckAll() {
	c.statuses.registry.cache.Range(func(key, value any) bool {
		c.statuses.registry.cache.Delete(key)
		return true
	})
}

// BroadcastStatus sends the same status to all watchers across all services.
// Useful for shutdown scenarios.
func (c *Coordinator) BroadcastStatus(status ServingStatus) {
	c.watchers.broadcast(status)
}

// GetMetrics returns runtime metrics for monitoring.
//
// Example:
//
//	metrics := coordinator.GetMetrics()
//	fmt.Println("Active watchers:", metrics["active_watchers"])
func (c *Coordinator) GetMetrics() map[string]any {
	metrics := make(map[string]any)

	metrics["active_watchers"] = c.watchers.activeWatchers.Load()
	metrics["list_calls_in_flight"] = c.statuses.listCallsInFlight.Load()

	cached := 0
	c.statuses.registry.cache.Range(func(key, value any) bool {
		cached++
		return true
	})
	metrics["cached_statuses"] = cached

	metrics["total_watchers"] = c.watchers.count()
	return metrics
}
