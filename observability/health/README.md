# health

```go
import "github.com/altessa-s/go-atlas/observability/health"
```

Package `health` provides transport-agnostic health check coordination and monitoring. The central type is `Coordinator`, which manages a registry of
named services, caches status results, and delivers changes to subscribers via sharded channels.

## Key types

| Type / Function       | Description                                                      |
|-----------------------|------------------------------------------------------------------|
| `Coordinator`         | Central health check manager with caching and subscriptions      |
| `Checker`             | Interface for health check implementations                       |
| `Registrar`           | Consumer port: `RegisterService` only, implemented by `Coordinator` |
| `Reporter`            | Consumer port: `Registrar` plus `NotifyStatusChange`             |
| `ScopedReporter` | Registers one owner with status and removal callbacks that expire on replacement |
| `DynamicReporter`     | Consumer port: `Reporter` plus `UnregisterService`               |
| `Func`                | Adapter to use a plain function as `Checker`                     |
| `Subscriber`          | Subscription for reactive status change notifications            |
| `ServingStatus`       | Status enum: `StatusServing`, `StatusNotServing`, `StatusDegraded`, `StatusUnknown`, `StatusServiceUnknown` |
| `RunHealthCheckCycle` | Single check cycle method for scheduler integration              |
| `Coordinator.Health`  | `Health(ctx) error`: nil while serving, else `ErrNotServing` — plugs into the gRPC health interceptor directly |

Status updates for a named service invalidate cached overall readiness. A push supersedes an overlapping poll for the same subscriber.
`RegisterScopedService` binds updates and removal to the registered owner, including concurrent replacement. `RegisterOwnedService` uses that
contract for a direct coordinator. For a decorated `Reporter`, it calls the supplied named methods, including `UnregisterService` when available.
A name-based reporter must serialize replacement with an outstanding update or removal. `DetachableChecker.Detach` releases its own registrations
through either coordinator registration method.

## Features

- Status caching with configurable TTL
- Sharded watcher channels for low lock contention
- Concurrent health check limiting
- Scheduler integration via `WithScheduler` / `WithCheckSchedule`

Each registration has its own identity. Removing a service during a check excludes it from overall health; replacing it makes that check report
not serving until the replacement is checked on a later call. `ListStatuses` omits removed services and reports `StatusNotServing` for replaced
services. Registry changes invalidate the service and overall caches, and checks spanning a registry change do not populate the cache.

`Health` and `CheckStatus` reuse results for the configured TTL. Set `WithStatusCacheTTL(0)` to disable caching. Each checker receives a context
bounded by `WithCheckTimeout` and any earlier caller deadline.

Canceling the subscription context or calling `Subscription.Close` releases that subscription. `Coordinator.Close` queues a final
`StatusNotServing` for active subscriptions before closing their channels. It drops the oldest queued update when the buffer is full.

Use `RegisterScopedService` to get an unregister function tied to that registration. Both this function and `UnregisterService` stop admitting
new checks for the removed registration and wait for its in-flight checks to finish. Invoke them outside the checker's `CheckHealth` method,
before closing the dependency it probes.

## Subpackages

| Package                                       | Description                                |
|-----------------------------------------------|--------------------------------------------|
| [factory](../../factory/observability/health) | Configuration-based `Coordinator` creation |

## Scheduler registration

`New` has no scheduling side effects. With `WithScheduler` and `WithCheckSchedule` configured, call `RegisterHealthChecks(ctx)` and handle its error
before starting the service. Only successful registration disables manual `RunHealthCheckCycle`; a failure can be retried on the same coordinator.
Repeated successful registration is a no-op. The factory performs this registration and returns an error if it fails.

```go
coordinator := health.New(health.WithScheduler(sched), health.WithCheckSchedule("@every 5s"))
defer coordinator.Close()
if err := coordinator.RegisterHealthChecks(ctx); err != nil {
    return err
}
```
