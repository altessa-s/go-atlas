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
| `Func`                | Adapter to use a plain function as `Checker`                     |
| `Subscriber`          | Subscription for reactive status change notifications            |
| `ServingStatus`       | Status enum: `Serving`, `NotServing`, `Degraded`, `Unknown`      |
| `RunHealthCheckCycle` | Single check cycle method for scheduler integration              |

## Features

- Status caching with configurable TTL
- Sharded watcher channels for low lock contention
- Concurrent health check limiting
- Scheduler integration via `WithScheduler` / `WithCheckSchedule`

## Subpackages

| Package              | Description                                            |
|----------------------|--------------------------------------------------------|
| [factory](./factory) | Configuration-based `Coordinator` creation             |

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
