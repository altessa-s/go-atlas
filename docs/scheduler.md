# Scheduler

```go
import "github.com/altessa-s/go-atlas/service/scheduler"
```

In-process persistent task scheduler with cron-based scheduling, priority-based dispatch,
and pluggable storage backends.

The scheduler runs periodic and deferred work — cache rebuilds, cleanup jobs, report
generation, metric aggregation — with crash recovery, concurrency control, and full
observability.

---

## Overview

| Capability              | Description                                                                   |
|-------------------------|-------------------------------------------------------------------------------|
| Cron scheduling         | Six-field cron expressions (with seconds) and descriptor syntax               |
| One-shot tasks          | Execute once at a specific time                                               |
| Priority dispatch       | Four priority levels with reserved high-priority concurrency slots            |
| Crash recovery          | Automatic detection and recovery of tasks stuck in `Running` state            |
| Concurrency strategies  | Static, environment-preset, memory-aware, adaptive, or custom function        |
| Pluggable storage       | In-memory, MongoDB, or Redis backends with a unified `Storage` interface      |
| Distributed scheduling  | Leader election ensures exactly-one execution across replicas                 |
| Observability           | Prometheus metrics, structured logging, execution history with pagination     |

## When to use

| Scenario                      | What the scheduler provides                                                  |
|-------------------------------|------------------------------------------------------------------------------|
| Periodic cache rebuilds       | Cron-based scheduling with crash recovery — rebuilds resume after restart    |
| Report / export generation    | One-shot tasks with priority dispatch — heavy jobs don't starve lighter work |
| Session / token cleanup       | Recurring purge with execution history and stale-task recovery               |
| Metric aggregation            | High-frequency tick interval with adaptive concurrency for I/O-bound work    |
| Webhook retry / outbox drain  | Priority-based dispatch — retries at higher priority without blocking work   |
| Distributed cron (multi-node) | Leader election ensures exactly-one execution across replicas                |

> If your work is fire-and-forget with no persistence requirement, a plain `time.Ticker`
> or goroutine is simpler. Reach for the scheduler when you need crash recovery, priority
> ordering, execution history, or distributed coordination.

---

## Quick start

### Minimal example

```go
package main

import (
	"context"
	"log"
	"log/slog"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
)

func main() {
	ctx := context.Background()

	store := memory.New(1000)

	sched := scheduler.New(store,
		scheduler.WithLogger(slog.Default()),
	)

	err := sched.Register(ctx, corescheduler.TaskConfig{
		ID:       "cleanup-expired-sessions",
		Schedule: "@every 5m",
		Func: func(ctx context.Context) error {
			// Your task logic.
			return nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := sched.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer sched.Stop(ctx)
}
```

### Configuration-driven (factory builder)

When you build the scheduler from YAML configuration, use the factory builder:

```go
import "github.com/altessa-s/go-atlas/service/scheduler/factory"

sched, err := factory.New(cfg.Scheduler).
	UseLogger(logger).
	UseMongoDb(db).
	UseLeaderElector(elector).
	UseCollector(collector).
	UseReadinessProbe(func() bool {
		return health.CheckHealth(ctx) == health.StatusServing
	}).
	Build()
if err != nil {
	return err
}
```

The builder selects the storage backend, maps the concurrency strategy to scheduler options,
and returns a ready-to-start `*scheduler.Scheduler`.

| Method              | Description                                                 |
|---------------------|-------------------------------------------------------------|
| `UseLogger`         | Structured logger for the scheduler and internal components |
| `UseDefaultLogger`  | Convenience — sets the logger to `slog.Default()`           |
| `UseLeaderElector`  | Leader elector for distributed scheduling                   |
| `UseMongoDb`        | MongoDB database. Required when `storage.type` is `mongodb` |
| `UseRedisClient`    | Redis client. Required when `storage.type` is `redis`       |
| `UseCollector`      | Metrics collector for Prometheus instrumentation            |
| `UseReadinessProbe` | Defers task dispatch until all subsystems signal readiness  |

---

## Configuration

### YAML reference

```yaml
scheduler:
  tickInterval: 1s
  historyRetention: 168h
  staleTaskTimeout: 30m

  concurrency:
    strategy: static                  # static | environment | memory-aware | adaptive
    maxTasks: 5                       # Static strategy only. 0 = unlimited
    reservedHighPrioritySlots: 2
    environment: io-bound             # Environment strategy only
    memoryAware:                      # Memory-aware strategy only
      lowMemoryMB: 256
      mediumMemoryMB: 512
      highMemoryMB: 1024
    adaptive:                         # Adaptive strategy only
      memoryLowThresholdMB: 256
      memoryMediumThresholdMB: 512
      highLoadThreshold: 0.8

  storage:
    type: memory                      # memory | mongodb | redis
    memory:
      maxHistoryPerTask: 1000
    mongodb:
      tasksCollection: scheduler_tasks
      historyCollection: scheduler_history
    redis:
      keyPrefix: scheduler
      historyTtl: 0
      maxHistoryPerTask: 1000
```

### Scheduler

| Field              | Type       | Default | Description                                                               |
|--------------------|------------|---------|---------------------------------------------------------------------------|
| `tickInterval`     | `duration` | `1s`    | Main loop evaluation interval. Lower = more precise, more CPU. Min: `1ms` |
| `historyRetention` | `duration` | `168h`  | How long execution history is retained before cleanup                     |
| `staleTaskTimeout` | `duration` | `30m`   | Duration after which a `Running` task is considered stale and recovered   |

### Concurrency

| Field                       | Type     | Default    | Description                                                         |
|-----------------------------|----------|------------|---------------------------------------------------------------------|
| `strategy`                  | `string` | `static`   | `static`, `environment`, `memory-aware`, or `adaptive`              |
| `maxTasks`                  | `int`    | `5`        | Static concurrency limit. `0` = unlimited. Used with `static` only  |
| `reservedHighPrioritySlots` | `int`    | `2`        | Slots reserved for `High` and `Critical` priority tasks             |
| `environment`               | `string` | `io-bound` | Environment preset. Used with `environment` only                    |

**Memory-aware** (required when `strategy: memory-aware`):

| Field            | Type     | Description                                         |
|------------------|----------|-----------------------------------------------------|
| `lowMemoryMB`    | `uint64` | Below this threshold (MB): concurrency = 1          |
| `mediumMemoryMB` | `uint64` | Below this threshold (MB): conservative concurrency |
| `highMemoryMB`   | `uint64` | Above this threshold (MB): aggressive concurrency   |

**Adaptive** (required when `strategy: adaptive`):

| Field                     | Type      | Description                                               |
|---------------------------|-----------|-----------------------------------------------------------|
| `memoryLowThresholdMB`    | `uint64`  | Below this (MB): concurrency reduced to 25% of base       |
| `memoryMediumThresholdMB` | `uint64`  | Below this (MB): concurrency reduced to 50% of base       |
| `highLoadThreshold`       | `float64` | System load above which concurrency scales down. Optional |

### Storage

See [Storage backends](#storage-backends) for backend-specific configuration.

---

## Tasks

### Registration

Register tasks by calling `Register` with a `TaskConfig`. Each task requires an `ID`,
a `Func`, and exactly one of `Schedule` (recurring) or `RunAt` (one-shot).

```go
// Recurring: standard six-field cron (with seconds).
sched.Register(ctx, corescheduler.TaskConfig{
	ID:       "aggregate-metrics",
	Schedule: "0 */5 * * * *",
	Priority: scheduler.TaskPriorityHigh,
	Func:     aggregateMetrics,
})

// Recurring: descriptor syntax.
sched.Register(ctx, corescheduler.TaskConfig{
	ID:       "daily-report",
	Schedule: "@daily",
	Func:     generateReport,
})

// One-shot: execute once at a specific time.
sched.Register(ctx, corescheduler.TaskConfig{
	ID:    "send-welcome-email",
	RunAt: time.Now().Add(10 * time.Minute),
	Func:  sendWelcomeEmail,
})
```

Supported cron descriptors: `@every <duration>`, `@hourly`, `@daily`, `@weekly`, `@monthly`.

### TaskConfig reference

| Field            | Type                              | Required              | Default    | Description                                                            |
|------------------|-----------------------------------|-----------------------|------------|------------------------------------------------------------------------|
| `ID`             | `string`                          | yes                   | --         | Unique task identifier                                                 |
| `Func`           | `func(ctx context.Context) error` | yes                   | --         | Function invoked on each execution                                     |
| `Schedule`       | `string`                          | one of Schedule/RunAt | --         | Cron expression or descriptor for recurring execution                  |
| `RunAt`          | `time.Time`                       | one of Schedule/RunAt | --         | Specific time for one-shot execution                                   |
| `Description`    | `string`                          | no                    | `""`       | Human-readable summary for listing endpoints                           |
| `Priority`       | `TaskPriority`                    | no                    | `Normal`   | Dispatch priority for concurrency slot allocation                      |
| `Timeout`        | `time.Duration`                   | no                    | `0` (none) | Maximum execution duration. Context is canceled on expiry              |
| `RunOnStart`     | `bool`                            | no                    | `false`    | Execute once immediately on registration, in addition to the schedule  |
| `DisableHistory` | `bool`                            | no                    | `false`    | Skip recording execution history. Useful for high-frequency tasks      |
| `Unmanaged`      | `bool`                            | no                    | `false`    | Exempt from `PauseTask` and `DisableTask`                              |
| `Meta`           | `map[string]string`               | no                    | `nil`      | Arbitrary key-value metadata for monitoring or management              |

### Re-registration

Calling `Register` with an existing task ID **merges** the configuration: schedule, priority,
and description are updated; execution state (`LastRunAt`, `Failures`) is preserved. This
lets you update a task's schedule without losing state.

### Lifecycle

```
                ┌──────────────────────────────────────────┐
                │                                          │
  Register ──> Active ──> Running ──> Active (recurring)   │
                │  ^                    │                   │
                │  │                    └──> Completed      │
                │  │                         (one-shot)     │
                v  │                                        │
              Paused                                        │
                                                            │
              Disabled <────────────────────────────────────┘
```

| Status      | Value | Description                                                                                   |
|-------------|-------|-----------------------------------------------------------------------------------------------|
| `Active`    | `1`   | Steady state. Dispatched when due                                                             |
| `Paused`    | `2`   | Suspended via `PauseTask`. Resume with `ResumeTask`. In-flight execution completes normally   |
| `Disabled`  | `3`   | Fully deactivated. Cannot be triggered manually or by schedule. Re-activate with `EnableTask` |
| `Running`   | `4`   | Currently executing                                                                           |
| `Completed` | `5`   | One-shot task finished. Re-register to reset                                                  |

### Management operations

```go
sched.PauseTask(ctx, "my-task")     // Prevent future dispatches
sched.ResumeTask(ctx, "my-task")    // Resume; recomputes next run time from now
sched.DisableTask(ctx, "my-task")   // Block both scheduled and manual execution
sched.EnableTask(ctx, "my-task")    // Re-enable a disabled task
sched.SkipNextRun(ctx, "my-task")   // Skip the next scheduled execution
sched.TriggerTask(ctx, "my-task")   // Immediate execution; respects concurrency limits
sched.Unregister(ctx, "my-task")    // Remove from scheduler and storage
```

---

## Concurrency control

The scheduler supports four concurrency strategies configurable via YAML or Go options.
All strategies support reserved high-priority slots and `Critical` priority bypass.

### Strategy comparison

| Strategy       | Mechanism              | Adapts at runtime | Best for                                        |
|----------------|------------------------|-------------------|-------------------------------------------------|
| `static`       | Semaphore-based        | No                | Predictable workloads with known capacity       |
| `environment`  | Preset profile         | No                | Quick setup matching deployment characteristics |
| `memory-aware` | Memory threshold-based | Yes               | Memory-sensitive workloads (ETL, batch)         |
| `adaptive`     | Memory + load factor   | Yes               | Variable workloads in shared infrastructure     |

### Static

Fixed upper bound enforced via semaphores. Default strategy.

```go
sched := scheduler.New(store,
	scheduler.WithMaxConcurrentTasks(10),
	scheduler.WithReservedHighPrioritySlots(2),
)
```

With `maxTasks: 10` and `reservedHighPrioritySlots: 2`, 8 slots are shared across all
priorities, and 2 are reserved for `High` and `Critical` tasks.

When `maxTasks` is `0`, all due tasks execute immediately with no limit.

```yaml
concurrency:
  strategy: static
  maxTasks: 10
  reservedHighPrioritySlots: 2
```

### Environment presets

Selects a concurrency limit from a predefined profile. The limit is computed once at
creation time.

```go
sched := scheduler.New(store,
	scheduler.WithEnvironment(concurrency.EnvironmentIOBound),
)
```

| Preset                         | Limit                  |
|--------------------------------|------------------------|
| `EnvironmentMemoryConstrained` | `1`                    |
| `EnvironmentCPUBound`          | `runtime.NumCPU()`     |
| `EnvironmentIOBound`           | `runtime.NumCPU() * 2` |
| `EnvironmentHighThroughput`    | `runtime.NumCPU() * 4` |
| `EnvironmentRateLimited`       | `3`                    |

```yaml
concurrency:
  strategy: environment
  environment: io-bound
```

### Memory-aware

Dynamically adjusts concurrency based on available system memory, evaluated on each tick.

```go
sched := scheduler.New(store,
	scheduler.WithConcurrencyLimitFunc(
		concurrency.MemoryAwareConcurrency(256, 512, 1024),
	),
)
```

Three thresholds control behavior:

| Available memory   | Concurrency behavior  |
|--------------------|-----------------------|
| < `lowMemoryMB`    | 1 (survival mode)     |
| < `mediumMemoryMB` | Conservative          |
| >= `highMemoryMB`  | Aggressive            |

```yaml
concurrency:
  strategy: memory-aware
  memoryAware:
    lowMemoryMB: 256
    mediumMemoryMB: 512
    highMemoryMB: 1024
```

### Adaptive

Combines memory pressure and system load into a single concurrency signal, evaluated on
each tick.

```go
sched := scheduler.New(store,
	scheduler.WithConcurrencyLimitFunc(
		concurrency.AdaptiveConcurrency(concurrency.AdaptiveConcurrencyConfig{
			MemoryLowThresholdMB:    256,
			MemoryMediumThresholdMB: 512,
			HighLoadThreshold:       0.8,
		}),
	),
)
```

| Condition                                    | Effect                             |
|----------------------------------------------|------------------------------------|
| Available memory < `MemoryLowThresholdMB`    | Concurrency reduced to 25% of base |
| Available memory < `MemoryMediumThresholdMB` | Concurrency reduced to 50% of base |
| System load > `HighLoadThreshold`            | Further scaled down by load factor |

```yaml
concurrency:
  strategy: adaptive
  adaptive:
    memoryLowThresholdMB: 256
    memoryMediumThresholdMB: 512
    highLoadThreshold: 0.8
```

### Custom function

For full control, provide a function evaluated on each tick:

```go
sched := scheduler.New(store,
	scheduler.WithConcurrencyLimitFunc(func() int {
		return runtime.NumCPU() * 2
	}),
)
```

When `WithConcurrencyLimitFunc` is set, it overrides `WithMaxConcurrentTasks`.

### Priority levels

| Priority               | Value | Slot behavior                                                |
|------------------------|-------|--------------------------------------------------------------|
| `TaskPriorityLow`      | `1`   | Executes only when no higher-priority tasks are waiting      |
| `TaskPriorityNormal`   | `2`   | Default. Uses shared concurrency slots                       |
| `TaskPriorityHigh`     | `3`   | Uses reserved slots plus the shared pool                     |
| `TaskPriorityCritical` | `4`   | Bypasses all concurrency limits. Always executes immediately |

### Concurrency observability

```go
sched.RunningTasksCount()        // Currently executing non-critical tasks
sched.AvailableSlots()           // Available concurrency slots (-1 if unlimited)
sched.AvailableHighPrioritySlots() // Reserved HP slots available (-1 if dynamic/unlimited)
```

---

## Storage backends

The scheduler persists task state and execution history to a pluggable `Storage` interface.

### Choosing a backend

| Capability         | Memory                 | MongoDB                          | Redis                          |
|--------------------|------------------------|----------------------------------|--------------------------------|
| Persistence        | No (process lifetime)  | Yes                              | Yes                            |
| Multi-node support | No                     | Yes                              | Yes                            |
| Filter push-down   | Client-side            | Server-side (BSON)               | Server-side (RediSearch)       |
| Index management   | N/A                    | `EnsureIndexes`                  | `EnsureIndexes`                |
| Best for           | Dev, test, single-node | Production with existing MongoDB | Production with existing Redis |

### Memory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

store := memory.New(1000)
```

In-process storage for development, testing, and single-node deployments.
All state is lost on restart.

```yaml
storage:
  type: memory
  memory:
    maxHistoryPerTask: 1000    # Per-task history cap (default: 1000)
```

**Characteristics:**
- Thread-safe via `sync.RWMutex`
- Deep-copy semantics — callers cannot mutate internal state
- Tasks iterated in ID-ascending order
- Cursor-seek uses binary search for efficient pagination

---

### MongoDB

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"

store := mongodb.New(db,
	mongodb.WithTasksCollection("scheduler_tasks"),
	mongodb.WithHistoryCollection("scheduler_history"),
)

if err := store.EnsureIndexes(ctx); err != nil {
	return err
}
```

Persistent storage for production multi-node deployments.

```yaml
storage:
  type: mongodb
  mongodb:
    tasksCollection: scheduler_tasks      # Default: scheduler_tasks
    historyCollection: scheduler_history  # Default: scheduler_history
```

**Indexes** (created by `EnsureIndexes`, idempotent):

| Collection | Index                            | Purpose                           |
|------------|----------------------------------|-----------------------------------|
| Tasks      | `(status ASC, next_run ASC)`     | Query due tasks efficiently       |
| Tasks      | `priority DESC`                  | Priority-based dispatch ordering  |
| History    | `(task_id ASC, start_time DESC)` | Per-task history listing          |
| History    | TTL on `end_time`                | Automatic expiration              |

<details>
<summary>Filter-to-BSON field mapping</summary>

Filter expressions use camelCase field names. The storage translates them to BSON:

| Filter field     | BSON field        |
|------------------|-------------------|
| `id`             | `_id`             |
| `lastRunAt`      | `last_run_at`     |
| `nextRunAt`      | `next_run_at`     |
| `lastRunId`      | `last_run_id`     |
| `skipNextRun`    | `skip_next_run`   |
| `disableHistory` | `disable_history` |
| `oneShot`        | `one_shot`        |
| `createdAt`      | `created_at`      |
| `updatedAt`      | `updated_at`      |

Fields not listed (`description`, `status`, `priority`, `schedule`, `failures`, `unmanaged`)
use the same name in both filter expressions and BSON.

</details>

**Characteristics:**
- Thread-safe via mongo-driver connection pool
- Filter expressions translated to BSON for server-side evaluation
- History pagination uses a compound cursor (`startedAt`, `_id`) for stable ordering
- `DeleteTask` removes both the task document and all associated history entries

---

### Redis

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"

store := redis.New(client,
	redis.WithKeyPrefix("scheduler"),
	redis.WithHistoryTTL(7 * 24 * time.Hour),
	redis.WithMaxHistoryPerTask(1000),
)

if err := store.EnsureIndexes(ctx); err != nil {
	return err
}
```

Persistent storage backed by Redis. **Requires RedisJSON and RediSearch modules.**

```yaml
storage:
  type: redis
  redis:
    keyPrefix: scheduler         # Default: scheduler
    historyTtl: 0                # Default: 0 (disabled)
    maxHistoryPerTask: 1000      # Default: 1000
```

**Key patterns:**

| Pattern                                   | Content                         |
|-------------------------------------------|---------------------------------|
| `{prefix}:task:{task_id}`                 | Task state (RedisJSON)          |
| `{prefix}:history:{task_id}:{history_id}` | History entry (RedisJSON)       |
| `{prefix}:idx:tasks`                      | RediSearch index over tasks     |
| `{prefix}:idx:history`                    | RediSearch index over history   |

<details>
<summary>RediSearch index schema</summary>

Created by `EnsureIndexes` (idempotent).

**Task index** (`{prefix}:idx:tasks`):

| JSON path           | Alias            | Type    | Sortable |
|---------------------|------------------|---------|----------|
| `$.id`              | `id`             | Tag     | yes      |
| `$.description`     | `description`    | Text    | no       |
| `$.status`          | `status`         | Numeric | yes      |
| `$.priority`        | `priority`       | Numeric | yes      |
| `$.schedule`        | `schedule`       | Tag     | no       |
| `$.last_run_at`     | `lastRunAt`      | Numeric | yes      |
| `$.next_run_at`     | `nextRunAt`      | Numeric | yes      |
| `$.failures`        | `failures`       | Numeric | no       |
| `$.skip_next_run`   | `skipNextRun`    | Tag     | no       |
| `$.disable_history` | `disableHistory` | Tag     | no       |
| `$.unmanaged`       | `unmanaged`      | Tag     | no       |
| `$.one_shot`        | `oneShot`        | Tag     | no       |
| `$.created_at`      | `createdAt`      | Numeric | yes      |
| `$.updated_at`      | `updatedAt`      | Numeric | yes      |

**History index** (`{prefix}:idx:history`):

| JSON path       | Alias        | Type    | Sortable |
|-----------------|--------------|---------|----------|
| `$.task_id`     | `taskId`     | Tag     | no       |
| `$.run_id`      | `runId`      | Tag     | no       |
| `$.started_at`  | `startedAt`  | Numeric | yes      |
| `$.ended_at`    | `endedAt`    | Numeric | yes      |
| `$.duration_ms` | `durationMs` | Numeric | no       |
| `$.success`     | `success`    | Tag     | no       |
| `$.error`       | `error`      | Text    | no       |

</details>

**Characteristics:**
- Thread-safe via `redis.UniversalClient`
- Filter expressions translated to RediSearch query syntax for server-side evaluation
- `FT.SEARCH` results capped at 10,000 entries
- `DeleteTask` removes the task key and all history keys in a single pipeline
- History trimming on `AddHistory` is best-effort — concurrent writers may temporarily exceed the cap

---

## Distributed scheduling

In multi-node deployments, use a leader elector to ensure only one instance dispatches tasks.
All instances persist state, but only the leader executes task functions.

```go
sched := scheduler.New(store,
	scheduler.WithLeaderElector(elector),
)

if sched.IsLeader() {
	// This instance is dispatching tasks.
}
```

When no `WithLeaderElector` is provided, `IsLeader` always returns `true`.

---

## Readiness probe

Defer task dispatch until all subsystems (databases, caches, message brokers) have finished
initializing. The probe is evaluated at the start of each tick — when it returns `false`,
the tick is skipped but the main loop keeps running so `Stop` works cleanly.

```go
sched := scheduler.New(store,
	scheduler.WithReadinessProbe(func() bool {
		return health.CheckHealth(ctx) == health.StatusServing
	}),
)
```

- `IsReady()` reports the current probe status
- `TriggerTask` returns `ErrNotReady` when the probe returns `false`
- When no probe is configured, the scheduler is always ready

---

## Querying tasks and history

### Iterators

```go
// All tasks.
for summary, err := range sched.Tasks(ctx) {
	if err != nil {
		return err
	}
	fmt.Printf("%s  status=%s  next_run=%d\n", summary.ID, summary.Status, summary.NextRunAt)
}

// Execution history for a specific task.
for entry, err := range sched.History(ctx, "my-task") {
	if err != nil {
		return err
	}
	fmt.Printf("run=%s  success=%v  duration=%dms\n", entry.RunID, entry.Success, entry.DurationMs)
}
```

### Paginated queries with filters

`TasksPaginated` and `HistoryPaginated` accept a filter expression and cursor-based
pagination. Filters are pushed down to the storage layer for server-side evaluation
(MongoDB, Redis) or evaluated client-side (memory).

```go
page := scheduler.PageRequest{Limit: 50}

result, err := sched.TasksPaginated(ctx, page, `status == 1 && priority >= 3`)
if err != nil {
	return err
}

for _, summary := range result.Items {
	fmt.Println(summary.ID, summary.Status)
}

// Next page.
if result.NextCursor != nil {
	next := scheduler.PageRequest{Limit: 50, Cursor: *result.NextCursor}
	result, err = sched.TasksPaginated(ctx, next, `status == 1 && priority >= 3`)
}
```

```go
// Paginated history with filter.
result, err := sched.HistoryPaginated(ctx, "my-task",
	scheduler.PageRequest{Limit: 20},
	`success == false`,
)
```

**Pagination constants:**

| Constant          | Value  | Description                    |
|-------------------|--------|--------------------------------|
| `DefaultPageSize` | `100`  | Applied when `Limit` is &le; 0 |
| `MaxPageSize`     | `1000` | Upper clamp for `Limit`        |

**Available filter fields:**

| Scope   | Fields                                                                                                                                               |
|---------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| Tasks   | `id`, `description`, `status`, `priority`, `schedule`, `lastRunAt`, `nextRunAt`, `skipNextRun`, `disableHistory`, `unmanaged`, `oneShot`, `failures` |
| History | `id`, `taskId`, `runId`, `startedAt`, `endedAt`, `durationMs`, `success`, `error`                                                                    |

---

## Scheduler lifecycle

```go
// Start: loads state, recovers stale tasks, begins tick/cleanup/recovery goroutines.
if err := sched.Start(ctx); err != nil {
	return err
}

// Runtime status.
sched.IsRunning()  // Main loop active
sched.IsLeader()   // Elected leader (always true without leader elector)
sched.IsReady()    // Readiness probe passing (always true without probe)

// Stop: cancels internal context, waits for in-flight tasks.
// Returns ctx.Err() if the provided context expires first.
if err := sched.Stop(ctx); err != nil {
	return err
}
```

- `Start` must be called exactly once
- `Stop` on an already-stopped scheduler is a no-op

---

## Observability

### Metrics

When a `metrics.Collector` is provided via `WithCollector`, the scheduler records Prometheus
metrics under the `scheduler` subsystem. When no collector is configured, a no-op
implementation is used and all operations are zero-cost.

| Metric                                  | Type      | Labels                | Description                                      |
|-----------------------------------------|-----------|-----------------------|--------------------------------------------------|
| `scheduler_tasks_dispatched_total`      | Counter   | `task_id`, `priority` | Task executions started                          |
| `scheduler_task_duration_seconds`       | Histogram | `task_id`, `priority` | Task execution duration                          |
| `scheduler_task_errors_total`           | Counter   | `task_id`             | Failed task executions                           |
| `scheduler_tasks_skipped_total`         | Counter   | `task_id`             | Executions skipped via `SkipNextRun`             |
| `scheduler_stale_tasks_recovered_total` | Counter   | --                    | Stale task recoveries                            |
| `scheduler_tick_duration_seconds`       | Histogram | --                    | Tick cycle duration                              |
| `scheduler_tasks_running`               | Gauge     | --                    | Currently executing tasks                        |
| `scheduler_tasks_registered`            | Gauge     | --                    | Registered tasks                                 |
| `scheduler_dispatch_lag_seconds`        | Histogram | `task_id`             | Delay between scheduled and actual execution     |
| `scheduler_storage_errors_total`        | Counter   | `op`                  | Storage operation failures                       |

---

## Error handling

All errors are exported as sentinel values. Use `errors.Is` to match.

| Error                  | Returned by                                                                          | Cause                                          |
|------------------------|--------------------------------------------------------------------------------------|------------------------------------------------|
| `ErrTaskNotFound`      | `PauseTask`, `ResumeTask`, `DisableTask`, `EnableTask`, `SkipNextRun`, `TriggerTask` | No task with the given ID in storage           |
| `ErrTaskNotRegistered` | `Unregister`, `TriggerTask`                                                          | Task ID not in the in-memory registration map  |
| `ErrTaskUnmanaged`     | `PauseTask`, `DisableTask`                                                           | Task has the `Unmanaged` flag set              |
| `ErrTaskNotPaused`     | `ResumeTask`                                                                         | Task status is not `Paused`                    |
| `ErrTaskNotDisabled`   | `EnableTask`                                                                         | Task status is not `Disabled`                  |
| `ErrTaskDisabled`      | `TriggerTask`                                                                        | Task status is `Disabled`                      |
| `ErrTaskCompleted`     | `PauseTask`, `ResumeTask`, `EnableTask`, `TriggerTask`                               | One-shot task already executed                 |
| `ErrNotReady`          | `TriggerTask`                                                                        | Readiness probe returned `false`               |
| `ErrScheduleConflict`  | `Register`                                                                           | Both `RunAt` and `Schedule` provided           |
| `ErrInvalidCursor`     | `TasksPaginated`, `HistoryPaginated`                                                 | Cursor malformed or filter changed since issue |

---

## API reference

### Types

| Type             | Description                                                                    |
|------------------|--------------------------------------------------------------------------------|
| `Scheduler`      | Core scheduler: register, dispatch, pause/resume/disable, query                |
| `Storage`        | Persistence interface (9 methods) implemented by every backend                 |
| `TaskState`      | Full persistent state: schedule, priority, timestamps, metadata                |
| `TaskSummary`    | Lightweight read-only view for listing endpoints                               |
| `TaskHistory`    | Single execution record: timing, success flag, error, run ID                   |
| `TaskStatus`     | Lifecycle enum: `Active`, `Paused`, `Disabled`, `Running`, `Completed`         |
| `TaskPriority`   | Dispatch priority: `Low`, `Normal`, `High`, `Critical`                         |
| `PageRequest`    | Cursor-based pagination input (limit + opaque cursor)                          |
| `PageResult[T]`  | Generic paginated response (items + next cursor)                               |

### Options

| Option                          | Default         | Description                                                     |
|---------------------------------|-----------------|-----------------------------------------------------------------|
| `WithTickInterval`              | `1s`            | Main loop evaluation interval                                   |
| `WithMaxConcurrentTasks`        | `0` (unlimited) | Static concurrency limit                                        |
| `WithConcurrencyLimitFunc`      | `nil`           | Dynamic concurrency function (overrides static limit)           |
| `WithEnvironment`               | --              | Preset concurrency profile                                      |
| `WithReservedHighPrioritySlots` | `2`             | Slots reserved for `High` and `Critical` priority tasks         |
| `WithHistoryRetention`          | `168h`          | Execution history retention before cleanup                      |
| `WithCleanupInterval`           | `1h`            | Interval between history cleanup runs                           |
| `WithStaleTaskTimeout`          | `30m`           | Timeout before recovering tasks stuck in `Running`              |
| `WithLeaderElector`             | `nil`           | Distributed leader elector -- only the leader dispatches        |
| `WithReadinessProbe`            | `nil`           | Pre-dispatch readiness check -- skips tick when `false`         |
| `WithLogger`                    | discard         | Structured logger for lifecycle and error events                |
| `WithCollector`                 | noop            | Metrics collector for Prometheus instrumentation                |

---

## Packages

| Package                                                   | Description                                             |
|-----------------------------------------------------------|---------------------------------------------------------|
| [factory](../service/scheduler/factory)                   | Configuration-driven `Scheduler` and `Storage` creation |
| [storages/memory](../service/scheduler/storages/memory)   | In-memory backend with deep-copy semantics              |
| [storages/mongodb](../service/scheduler/storages/mongodb) | MongoDB backend with indexed queries                    |
| [storages/redis](../service/scheduler/storages/redis)     | Redis backend (RedisJSON + RediSearch)                  |
