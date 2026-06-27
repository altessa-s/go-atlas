# scheduler

```go
import "github.com/altessa-s/go-atlas/service/scheduler"
```

Package `scheduler` provides a persistent task scheduler with cron-based scheduling, priority dispatch,
and pluggable storage backends. Tasks are registered with functions, executed on schedule, and their
state is durably persisted so that incomplete executions are recovered after a crash. Supports static
and dynamic concurrency limits, distributed leader election, one-shot and recurring tasks, and
cursor-based paginated listing with CEL filter push-down.

## Key types

| Type / Interface | Description                                                                    |
|------------------|--------------------------------------------------------------------------------|
| `Scheduler`      | Core scheduler: register tasks, dispatch on tick, pause/resume/disable         |
| `Storage`        | Persistence interface (10 methods) implemented by every backend                |
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
| `WithStaleTaskTimeout`          | 30min     | Timeout before recovering tasks stuck in Running state        |
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
| `ErrTaskCompleted`     | Pause, Resume, Enable, TriggerTask -- one-shot task already finished |
| `ErrScheduleConflict`  | Register -- both RunAt and Schedule were provided simultaneously     |

## Single execution

In a multi-node deployment a `WithLeaderElector` keeps one instance dispatching, but leadership is only a throughput optimization. Each run is claimed
through `Storage.ClaimRun` — a single atomic compare-and-swap (`active → running`, fenced on the occurrence's `next_run_at`) — so even if two
instances believe they are leader during an election split-brain, exactly one claim wins and a task function runs at most once per occurrence. See
[docs/service/scheduler.md](../../docs/service/scheduler.md#single-execution-is-enforced-at-the-storage-layer).

## Subpackages

| Package                                | Description                                                     |
|----------------------------------------|-----------------------------------------------------------------|
| [factory](./factory)                   | Configuration-based Scheduler and Storage creation              |
| [storages/memory](./storages/memory)   | In-memory backend for dev/test with deep-copy semantics         |
| [storages/mongodb](./storages/mongodb) | MongoDB-backed persistent storage with indexed queries          |
| [storages/redis](./storages/redis)     | Redis (RedisJSON + RediSearch) persistent storage               |
