# scheduler

```go
import "github.com/altessa-s/go-atlas/service/scheduler"
```

Package `scheduler` provides a persistent task scheduler with cron-based scheduling, priority dispatch, and pluggable
storage backends. Tasks are registered with functions, executed on schedule, and their state is durably persisted so
that incomplete executions are recovered after a crash. Supports static and dynamic concurrency limits, distributed
leader election, one-shot and recurring tasks, and cursor-based paginated listing with CEL filter push-down.

## Usage

```go
storage := memory.New(100)
s := scheduler.New(storage,
    scheduler.WithTickInterval(time.Second),
    scheduler.WithMaxConcurrentTasks(10),
)

if err := s.Start(ctx); err != nil {
    log.Fatal(err)
}
defer s.Stop(ctx)

err := s.Register(ctx, corescheduler.TaskConfig{
    ID:       "cleanup",
    Schedule: "@every 5m",
    Priority: corescheduler.TaskPriorityHigh,
    Func: func(ctx context.Context) error {
        return doWork(ctx)
    },
})
```

### Dynamic concurrency

```go
s := scheduler.New(storage,
    scheduler.WithConcurrencyLimitFunc(
        concurrency.AdaptiveConcurrency(concurrency.AdaptiveConcurrencyConfig{
            MemoryLowThresholdMB:    256,
            MemoryMediumThresholdMB: 512,
        }),
    ),
)
```

### Distributed leader election

```go
le, _ := leadelect.NewWithNats(conn, cfg)
s := scheduler.New(storage, scheduler.WithLeaderElector(le))
```

## Key types

| Type / Interface | Description                                                                                     |
|------------------|-------------------------------------------------------------------------------------------------|
| `Scheduler`      | Core scheduler: register tasks, dispatch on tick, pause/resume/disable, trigger manual runs     |
| `Storage`        | Persistence interface (9 methods) implemented by every backend, including paginated queries      |
| `TaskState`      | Full persistent state of a task including schedule, priority, timestamps (embeds `TaskSummary`) |
| `TaskSummary`    | Lightweight read-only view returned by listing endpoints, omits internal scheduler fields       |
| `TaskHistory`    | Record of a single execution: start/end time, success flag, error message, run ID              |
| `TaskStatus`     | Lifecycle enum: Active, Paused, Disabled, Running, Completed                                    |
| `TaskPriority`   | Dispatch priority: Low, Normal, High, Critical — determines semaphore slot allocation           |
| `PageRequest`    | Cursor-based pagination input with limit and opaque cursor string                               |
| `PageResult[T]`  | Generic paginated response carrying items slice and next-page cursor                            |

## Options

| Option                          | Default     | Description                                                                    |
|---------------------------------|-------------|--------------------------------------------------------------------------------|
| `WithTickInterval`              | 1s          | Main loop evaluation interval — how often the scheduler checks for due tasks   |
| `WithMaxConcurrentTasks`        | 0 (no cap)  | Static concurrency limit; 0 means unlimited with per-tick dispatch cap         |
| `WithConcurrencyLimitFunc`      | nil         | Dynamic concurrency function, overrides static limit when set                  |
| `WithReservedHighPrioritySlots` | 2           | Semaphore slots reserved exclusively for High and Critical priority tasks      |
| `WithHistoryRetention`          | 7 days      | How long to keep history entries before periodic cleanup removes them           |
| `WithCleanupInterval`           | 1h          | Interval between history cleanup runs that prune expired entries               |
| `WithStaleTaskTimeout`          | 30min       | Timeout before recovering tasks stuck in Running state after holder failure     |
| `WithLeaderElector`             | nil         | Distributed leader elector — only the leader dispatches tasks                  |
| `WithLogger`                    | discard     | Structured logger (`*slog.Logger`) for scheduler lifecycle and error events    |
| `WithEnvironment`               | --          | Preset concurrency profile tuned for a named environment (dev, staging, prod)  |

## Errors

| Error                  | Returned by                                                                       |
|------------------------|-----------------------------------------------------------------------------------|
| `ErrTaskNotFound`      | PauseTask, ResumeTask, DisableTask, EnableTask, SkipNextRun — task ID not in store|
| `ErrTaskNotRegistered` | Unregister, TriggerTask — task exists in storage but has no registered function   |
| `ErrTaskUnmanaged`     | PauseTask, DisableTask — the task was not registered through this scheduler       |
| `ErrTaskNotPaused`     | ResumeTask — cannot resume a task that is not in Paused status                    |
| `ErrTaskNotDisabled`   | EnableTask — cannot enable a task that is not in Disabled status                  |
| `ErrTaskDisabled`      | TriggerTask — cannot trigger manual execution of a disabled task                  |
| `ErrTaskCompleted`     | Pause, Resume, Enable, TriggerTask — one-shot task already finished               |
| `ErrScheduleConflict`  | Register — both RunAt (one-shot) and Schedule (cron) were provided simultaneously |

## Subpackages

| Package                                    | Description                                                                  |
|--------------------------------------------|------------------------------------------------------------------------------|
| [factory](./factory)                       | Configuration-based Scheduler and Storage creation from config objects       |
| [storages/memory](./storages/memory)       | In-memory backend for dev/test with deep-copy semantics and binary search    |
| [storages/mongodb](./storages/mongodb)     | MongoDB-backed persistent storage with indexed queries and CEL translation   |
| [storages/redis](./storages/redis)         | Redis (RedisJSON + RediSearch) persistent storage with server-side filtering |
