# health

```go
import "github.com/altessa-s/go-atlas/observability/health"
```

Package `health` provides transport-agnostic health check coordination and monitoring. The central type is
`Coordinator`, which manages a registry of named services, caches status results, and delivers changes to subscribers.

## Usage

```go
coordinator := health.New()

coordinator.RegisterService("database", health.Func(func(ctx context.Context) health.ServingStatus {
    if err := db.PingContext(ctx); err != nil {
        return health.StatusNotServing
    }
    return health.StatusServing
}))

status := coordinator.CheckStatus(ctx, "database")
```

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
