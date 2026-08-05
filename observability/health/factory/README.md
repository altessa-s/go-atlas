# factory

```go
import "github.com/altessa-s/go-atlas/observability/health/factory"
```

Package `factory` provides a fluent builder for creating health coordinators from configuration.
`CoordinatorBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
coordinator, err := factory.New(cfg.Health).
    UseLogger(logger).
    UseScheduler(scheduler). // optional; drives the periodic check cycle
    Build()
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `CoordinatorBuilder` for the given health config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseScheduler` | Sets the task scheduler that drives the periodic health check cycle |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the health coordinator |

## Check cycle

The check cycle is what re-evaluates watched services and notifies their watchers; watchers themselves are push-based and never poll. It runs only
when a scheduler is supplied — `health.healthCheckInterval` alone has no effect, because there is nothing to drive it.

The config expresses the cadence as a duration while the coordinator takes a cron-style expression, so the builder renders it as the scheduler's
`@every` descriptor: `healthCheckInterval: 5s` registers the `health-check` task with schedule `@every 5s`. A zero interval registers no task.
