# scheduler

```go
import "github.com/altessa-s/go-atlas/service/scheduler"
```

Package `scheduler` provides a persistent task scheduler with cron-based scheduling, priority dispatch, and pluggable storage backends. Tasks are
registered with functions, executed on schedule, and their state is durably persisted so that incomplete executions are recovered after a crash.
Supports static and dynamic concurrency limits, distributed leader election, one-shot and recurring tasks, and cursor-based paginated listing with CEL
filter push-down.

## Key types

| Type / Interface | Description                                                                    |
|------------------|--------------------------------------------------------------------------------|
| `Scheduler`      | Core scheduler: register tasks, dispatch on tick, pause/resume/disable         |
| `Storage`        | Persistence interface (15 methods) implemented by every backend                |
| `TaskState`      | Full persistent state of a task including schedule, priority, timestamps       |
| `TaskSummary`    | Lightweight read-only view returned by listing endpoints                       |
| `TaskHistory`    | Record of a single execution: start/end time, success flag, error, run ID     |
| `TaskStatus`     | Lifecycle enum: Active, Paused, Disabled, Running, Completed                   |
| `TaskPriority`   | Dispatch priority: Low, Normal, High, Critical for semaphore slot allocation   |
| `PageRequest`    | Cursor-based pagination input with limit and opaque cursor string              |
| `PageResult[T]`  | Generic paginated response carrying items slice and next-page cursor           |

## Options

| Option                          | Default   | Description                                                   |
|---------------------------------|-----------|---------------------------------------------------------------|
| `WithTickInterval`              | 1s        | Main loop evaluation interval for checking due tasks          |
| `WithMaxConcurrentTasks`        | 0 (none)  | Static concurrency limit; 0 means unlimited                   |
| `WithConcurrencyLimitFunc`      | nil       | Dynamic concurrency function, overrides static limit          |
| `WithReservedHighPrioritySlots` | 2         | Semaphore slots reserved for High and Critical priority tasks |
| `WithHistoryRetention`          | 7 days    | How long to keep history entries before cleanup               |
| `WithCleanupInterval`           | 1h        | Interval between history cleanup runs                         |
| `WithStaleTaskTimeout`          | 30min     | Run lease (min 5s); runs whose lease expired are recovered    |
| `WithInstanceID`                | random    | Owner ID stamped on runs; must be unique per live instance    |
| `WithStorageTimeout`            | 10s       | Per-operation deadline for scheduler-owned storage calls      |
| `WithLeaderElector`             | nil       | Distributed leader elector -- only the leader dispatches      |
| `WithLogger`                    | discard   | Structured logger for scheduler lifecycle and error events    |
| `WithEnvironment`               | --        | Preset concurrency profile for a named environment            |

## Errors

| Error                  | Returned by                                                          |
|------------------------|----------------------------------------------------------------------|
| `ErrTaskNotFound`      | PauseTask, ResumeTask, DisableTask, EnableTask, SkipNextRun         |
| `ErrTaskNotRegistered` | Unregister, TriggerTask -- task exists but has no registered func    |
| `ErrTaskUnmanaged`     | PauseTask, DisableTask -- task was not registered through this       |
| `ErrTaskNotPaused`     | ResumeTask -- cannot resume a task that is not in Paused status      |
| `ErrTaskNotDisabled`   | EnableTask -- cannot enable a task that is not in Disabled status    |
| `ErrTaskDisabled`      | TriggerTask -- cannot trigger manual execution of a disabled task    |
| `ErrTaskCompleted`     | Pause, Resume, Disable, Enable, TriggerTask -- one-shot task finished |
| `ErrScheduleConflict`  | Register -- both RunAt and Schedule were provided simultaneously     |
| `ErrTaskAlreadyDispatched` | TriggerTask -- a dispatch for this task is already queued or running |
| `ErrNotReady`          | TriggerTask -- the configured readiness probe returned false         |
| `ErrConcurrentUpdate`  | Register and management methods -- concurrent writes won `MaxUpdateAttempts` times; retry |

## Single execution

In a multi-node deployment a `WithLeaderElector` keeps one instance dispatching, but leadership is only a throughput optimization. Each run is claimed
through `Storage.ClaimRun` — a single atomic compare-and-swap (`active → running`, no unfinished run, fenced on the occurrence's `next_run_at`) —
so even if two instances believe they are leader during an election split-brain, exactly one claim wins. `Register` creates new tasks with the
atomic insert-if-absent `Storage.CreateTask`, so concurrent registration cannot overwrite a claimed state. Stale recovery, at startup and
periodically, resets only runs whose owner lease has expired or that this instance abandoned; a run still executing on a live instance is never
handed to a second executor. Effects that must never repeat across a network partition longer than the lease still need idempotency or external
fencing. See
[docs/service/scheduler.md](../../docs/service/scheduler.md#single-execution-what-the-storage-layer-enforces).

## Run leases

Run IDs name the executing instance (`<instance-id>/<random>`). `Storage.ClaimRun(ctx, id, RunClaim)` stores the first lease with the claim and fences
on both `NextRunAt` and `RunAt`; the owner then calls `Storage.RenewRun(ctx, id, runID, leaseUntil)` every third of the lease (`WithStaleTaskTimeout`,
at least 5s); it sets `TaskState.RunLeaseUntil` and increments the revision only while `runID` still owns an unfinished run (`LastRunID == runID`,
`RunStartedAt != 0`). `Storage.CreateTask` inserts a task with revision one only when its ID is absent and otherwise reports `false` without writing.
Custom storage implementations must perform both operations, and the `RunStartedAt == 0` condition of `ClaimRun`, atomically.

## Subpackages

| Package                                | Description                                                     |
|----------------------------------------|-----------------------------------------------------------------|
| [factory](./factory)                   | Configuration-based Scheduler and Storage creation              |
| [storages/memory](./storages/memory)   | In-memory backend for dev/test with deep-copy semantics         |
| [storages/mongodb](./storages/mongodb) | MongoDB-backed persistent storage with indexed queries          |
| [storages/redis](./storages/redis)     | Redis (RedisJSON + RediSearch) persistent storage               |

## Atomic finalization

`Storage.FinishRun(ctx, id, runID, result)` atomically commits execution results only while that run still owns an unfinished execution. Stale and
repeated finishes return `false` without writing. Finalization updates execution fields without replacing task configuration, preserves concurrent
pause/disable decisions, and advances the next occurrence only if the schedule still matches. Custom storage implementations must implement this
operation atomically; a `GetTask` check followed by `UpsertTask` does not satisfy the contract.

A finished one-shot run always leaves the task `Completed` with `NextRunAt` zero, even if the task was paused, disabled or resumed while it ran.
Re-registering a completed one-shot task with the same `RunAt` keeps it completed; only a different `RunAt` schedules a new run (`TaskState.RunAt`
records the registered occurrence). `RunResult.RunAt` names the occurrence a run executed, so a one-shot task re-registered for another `RunAt`
during the run is not completed by it and keeps the new occurrence. Custom storages must apply both rules in `FinishRun`.

## Fenced replacement

`Storage.ReplaceTaskIf(ctx, state, expect)` replaces a task only while its `Status`, `NextRunAt`, `LastRunID`, `RunStartedAt` and `Revision` still equal
the `TaskFence` the caller read (`scheduler.FenceOf`). Every storage write — `UpsertTask`, `CreateTask`, `ClaimRun`, `RenewRun`, `FinishRun`,
`ReplaceTaskIf` — increments `TaskState.Revision` atomically and ignores the caller's value, so any concurrent change, including one that leaves the run
fields untouched (metadata, `SkipNextRun`, a lease renewal), fails the fence. Stale-task recovery and `SkipNextRun` handling write through it, so a run
that finished or was re-claimed after the scheduler read the task is never overwritten. Absent fields compare equal to their zero values. Like
`FinishRun`, custom storage implementations must perform the comparison and the write atomically.
