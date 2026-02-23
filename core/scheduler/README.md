# scheduler

```go
import "github.com/altessa-s/go-atlas/core/scheduler"
```

Package `scheduler` defines the core interface and value types for task scheduling. Acts as a dependency-inversion boundary: subsystems register periodic or one-shot tasks via `TaskRegistrar` without importing the concrete scheduler implementation.

## Usage

```go
err := registrar.Register(ctx, scheduler.TaskConfig{
    ID:       "refresh-cache",
    Schedule: "@every 5m",
    Func: func(ctx context.Context) error {
        return refreshCache(ctx)
    },
    Priority: scheduler.TaskPriorityNormal,
    Timeout:  30 * time.Second,
})
```

## Interface

| Type            | Description                                              |
|-----------------|----------------------------------------------------------|
| `TaskRegistrar` | Interface for registering tasks (satisfied by service impl) |
| `TaskFunc`      | `func(ctx context.Context) error`                        |

## TaskConfig

| Field            | Description                                             |
|------------------|---------------------------------------------------------|
| `ID`             | Unique task identifier (required)                       |
| `Description`    | Human-readable summary                                  |
| `Schedule`       | Cron expression or descriptor (`@every 5m`, `@hourly`)  |
| `RunAt`          | One-shot execution time (mutually exclusive with Schedule) |
| `Func`           | Task function (required)                                |
| `Priority`       | Dispatch priority (default: Normal)                     |
| `Timeout`        | Per-execution deadline (zero = no timeout)               |
| `RunOnStart`     | Execute once immediately on registration                |
| `DisableHistory` | Skip execution-history recording                        |
| `Unmanaged`      | Exempt from pause/disable management                    |
| `Meta`           | Arbitrary key-value metadata                            |

## Priority levels

| Constant                | Value | Behavior                                       |
|-------------------------|-------|------------------------------------------------|
| `TaskPriorityLow`      | 1     | Runs only when no higher-priority tasks wait   |
| `TaskPriorityNormal`   | 2     | Default for most workloads                     |
| `TaskPriorityHigh`     | 3     | Reserved slots, preempts Normal/Low            |
| `TaskPriorityCritical` | 4     | Bypasses concurrency limits entirely           |
