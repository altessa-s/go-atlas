# scheduler

```go
import "github.com/altessa-s/go-atlas/core/scheduler"
```

Package `scheduler` defines the core interface and value types for task scheduling. Acts as a dependency-inversion boundary — subsystems
register periodic or one-shot tasks via `TaskRegistrar` without importing the concrete scheduler implementation from `service/scheduler`.

## Interface

| Type            | Description                                              |
|-----------------|----------------------------------------------------------|
| `TaskRegistrar` | Interface for registering tasks (satisfied by service impl) |
| `TaskFunc`      | `func(ctx context.Context) error`                        |
| `ManagedTask`   | Per-cycle guard: single-flight execution + scheduler-managed flag |

## ManagedTask

Subsystems that expose both a scheduler task and a manual `RunXxx` entry point embed a `ManagedTask` per cycle. `SchedulerFunc(fn)` marks
the cycle scheduler-managed and returns `fn` wrapped with the single-flight guard; `Run(ctx, fn)` is the manual entry point and returns
`ErrSchedulerManaged` once the cycle is scheduler-managed; `TryRun(ctx, fn)` applies only the single-flight guard (for internal poll
loops); `MarkRegistered()` / `Registered()` flip and read the flag directly.

| Symbol                | Description                                                          |
|-----------------------|----------------------------------------------------------------------|
| `ErrSchedulerManaged` | Returned by `Run` when the cycle has been handed to a scheduler      |
| `SchedulerFunc(fn)`   | Mark managed; wrap `fn` with the single-flight guard for `TaskConfig.Func` |
| `Run(ctx, fn)`        | Manual entry point; guarded and blocked once scheduler-managed       |
| `TryRun(ctx, fn)`     | Single-flight only; overlapping calls return nil without running     |
| `MarkRegistered()`    | Flip the scheduler-managed flag without wrapping a function          |
| `Registered()`        | Report whether the cycle is scheduler-managed                        |

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
