# Scheduler

```go
import "github.com/altessa-s/go-atlas/service/scheduler"
```

In-process persistent task scheduler with cron-based scheduling, priority-based dispatch, and pluggable storage backends.

The scheduler runs periodic and deferred work — cache rebuilds, cleanup jobs, report generation, metric aggregation — with crash recovery,
concurrency control, and full observability.

---

## Overview

| Capability             | Description                                                                                                |
|------------------------|------------------------------------------------------------------------------------------------------------|
| Cron scheduling        | Six-field cron expressions (with seconds) and descriptor syntax                                            |
| One-shot tasks         | Execute once at a specific time                                                                            |
| Priority dispatch      | Four priority levels with reserved high-priority concurrency slots                                         |
| Crash recovery         | Automatic detection and recovery of tasks stuck in `Running` state                                         |
| Concurrency strategies | Static, environment-preset, memory-aware, adaptive, or custom function                                     |
| Pluggable storage      | In-memory, MongoDB, Redis, or SQL backends with a unified `Storage` interface                              |
| Distributed scheduling | Leader-coordinated dispatch + atomic claims ([details](#single-execution-what-the-storage-layer-enforces)) |
| Observability          | Prometheus metrics, structured logging, execution history with pagination                                  |

## When to use

| Scenario                      | What the scheduler provides                                                  |
|-------------------------------|------------------------------------------------------------------------------|
| Periodic cache rebuilds       | Cron-based scheduling with crash recovery — rebuilds resume after restart    |
| Report / export generation    | One-shot tasks with priority dispatch — heavy jobs don't starve lighter work |
| Session / token cleanup       | Recurring purge with execution history and stale-task recovery               |
| Metric aggregation            | High-frequency tick interval with adaptive concurrency for I/O-bound work    |
| Webhook retry / outbox drain  | Priority-based dispatch — retries at higher priority without blocking work   |
| Distributed cron (multi-node) | Leader-coordinated dispatch + atomic claims; effects must be idempotent      |

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

	store, err := memory.New(1000)
	if err != nil {
		log.Fatal(err)
	}

	sched := scheduler.New(store,
		scheduler.WithLogger(slog.Default()),
	)

	err = sched.Register(ctx, corescheduler.TaskConfig{
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

The builder selects the storage backend, maps the concurrency strategy to scheduler options, and returns a ready-to-start `*scheduler.Scheduler`.

| Method              | Description                                                 |
|---------------------|-------------------------------------------------------------|
| `UseLogger`         | Structured logger for the scheduler and internal components |
| `UseDefaultLogger`  | Convenience — sets the logger to `slog.Default()`           |
| `UseLeaderElector`  | Leader elector for distributed scheduling                   |
| `UseMongoDb`        | MongoDB database. Required when `storage.type` is `mongodb` |
| `UseRedisClient`    | Redis client. Required when `storage.type` is `redis`       |
| `UseSQLDB`          | `*sql.DB` handle. Required when `storage.type` is `sql`     |
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
  instanceId: scheduler-0             # Optional. Unique per instance; default: random per process

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
    type: memory                      # memory | mongodb | redis | sql
    memory:
      maxHistoryPerTask: 1000
    mongodb:
      tasksCollection: scheduler_tasks
      historyCollection: scheduler_history
    redis:
      keyPrefix: scheduler
      historyTtl: "0s"
      maxHistoryPerTask: 1000
    sql:
      dialect: postgres               # postgres | mysql (MySQL 8.0+, MariaDB 10.6+)
      tasksTable: scheduler_tasks
      historyTable: scheduler_history
      ensureSchema: false             # true: the factory runs EnsureSchema while building
```

### Scheduler

| Field              | Type       | Default | Description                                                               |
|--------------------|------------|---------|---------------------------------------------------------------------------|
| `tickInterval`     | `duration` | `1s`    | Main loop evaluation interval. Lower = more precise, more CPU. Min: `1ms` |
| `historyRetention` | `duration` | `168h`  | How long execution history is retained before cleanup                     |
| `staleTaskTimeout` | `duration` | `30m`   | Run lease (min `5s`); runs whose lease expired are recovered              |
| `instanceId`       | `string`   | random  | Owner ID stamped on runs; unique per instance sharing a storage (max 128) |

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

Register tasks by calling `Register` with a `TaskConfig`. Each task requires an `ID`, a `Func`, and exactly one of `Schedule` (recurring) or `RunAt`
(one-shot).

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

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `ID` | `string` | yes | -- | Unique task identifier |
| `Func` | `func(ctx context.Context) error` | yes | -- | Function invoked on each execution |
| `Schedule` | `string` | one of Schedule/RunAt | -- | Cron expression or descriptor for recurring execution |
| `RunAt` | `time.Time` | one of Schedule/RunAt | -- | Specific time for one-shot execution; must be after the Unix epoch |
| `Description` | `string` | no | `""` | Human-readable summary for listing endpoints |
| `Priority` | `TaskPriority` | no | `Normal` | Dispatch priority for concurrency slot allocation |
| `Timeout` | `time.Duration` | no | `0` (none) | Maximum execution duration. Context is canceled on expiry |
| `RunOnStart` | `bool` | no | `false` | Execute once immediately on registration, in addition to the schedule |
| `DisableHistory` | `bool` | no | `false` | Skip recording execution history. Useful for high-frequency tasks |
| `Unmanaged` | `bool` | no | `false` | Exempt from `PauseTask` and `DisableTask` |
| `Meta` | `map[string]string` | no | `nil` | Arbitrary key-value metadata for monitoring or management |

### Re-registration

Calling `Register` with an existing task ID **merges** the configuration: schedule, priority, and description are updated; execution state (`LastRunAt`,
`Failures`) is preserved. This lets you update a task's schedule without losing state.

A one-shot task is completed once its single run finishes — even if it was paused, disabled or resumed while that run executed — and a
completed one-shot task stays completed when it is re-registered with the same `RunAt` (compared at second precision). Every instance and every
restart can therefore register the same one-shot task safely. Only a different `RunAt` resets it to `Active` and schedules a new run; for a task
completed before the scheduler persisted `RunAt`, only a `RunAt` later than its last run's start does. A one-shot task re-registered for a different
`RunAt` while a run executes is not completed by that run: it returns to `Active` with the new occurrence still scheduled. `PauseTask`,
`ResumeTask`, `DisableTask` and `EnableTask` reject a completed task with `ErrTaskCompleted`.

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
| `Completed` | `5`   | One-shot task finished. Re-register with a different `RunAt` to run it again                  |

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

The scheduler supports four concurrency strategies configurable via YAML or Go options. All strategies support reserved high-priority slots and
`Critical` priority bypass.

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

With `maxTasks: 10` and `reservedHighPrioritySlots: 2`, 8 slots are shared across all priorities, and 2 are reserved for `High` and `Critical` tasks.

When `maxTasks` is `0`, all due tasks execute immediately with no limit.

```yaml
concurrency:
  strategy: static
  maxTasks: 10
  reservedHighPrioritySlots: 2
```

### Environment presets

Selects a concurrency limit from a predefined profile. The limit is computed once at creation time.

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

Combines memory pressure and system load into a single concurrency signal, evaluated on each tick.

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

| Capability         | Memory                 | MongoDB                          | Redis                          | SQL                                          |
|--------------------|------------------------|----------------------------------|--------------------------------|----------------------------------------------|
| Persistence        | No (process lifetime)  | Yes                              | Yes                            | Yes                                          |
| Multi-node support | No                     | Yes                              | Yes                            | Yes                                          |
| Filter push-down   | Client-side            | Server-side (BSON)               | Numeric ranges; rest on client | Server-side (SQL `WHERE`)                    |
| Index management   | N/A                    | `EnsureIndexes`                  | `EnsureIndexes`                | `EnsureSchema`                               |
| Best for           | Dev, test, single-node | Production with existing MongoDB | Production with existing Redis | Production with PostgreSQL, MySQL or MariaDB |

### Memory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

store, err := memory.New(1000)
if err != nil {
    return err
}
```

In-process storage for development, testing, and single-node deployments. All state is lost on restart.

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

| Collection | Index                                       | Purpose                                                  |
|------------|---------------------------------------------|----------------------------------------------------------|
| Tasks      | `(status ASC, next_run_at ASC)`             | Query due tasks efficiently                              |
| Tasks      | `priority DESC`                             | Priority-based dispatch ordering                         |
| History    | `(task_id ASC, started_at DESC, _id DESC)`  | Per-task history listing (`History`, `HistoryPaginated`) |
| History    | `ended_at ASC`                              | Range delete in `CleanupHistory` (no TTL index)          |

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

Fields not listed (`description`, `status`, `priority`, `schedule`, `failures`, `unmanaged`) use the same name in both filter expressions and BSON.

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
    historyTtl: "0s"             # Default: 0s (disabled)
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
- Filter expressions are evaluated on the client with the memory backend's evaluator: RediSearch folds case on TAG fields, tokenizes TEXT fields
  and has no `endsWith`/`matches`/`size()`, so it cannot evaluate CEL exactly. Comparisons of `status`, `priority` and `failures` (history:
  `startedAt`, `endedAt`, `durationMs`) are also pushed down as numeric ranges to narrow the scan
- History lookups match the task ID exactly (the `taskId` TAG query folds case; entries of a task whose ID differs only by case are skipped)
- `FT.SEARCH` results are fetched in pages of 1,000 until every match is read — no fixed result cap
- `DeleteTask` removes the task key and all history keys in a single pipeline
- History trimming on `AddHistory` is best-effort — concurrent writers may temporarily exceed the cap

### SQL

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"

db, _ := sql.Open("pgx", dsn) // any database/sql driver; go-atlas ships none
store, err := sqldb.New(db, sqldb.DialectPostgres,
	sqldb.WithTasksTable("scheduler_tasks"),
	sqldb.WithHistoryTable("scheduler_history"),
)
if err != nil {
	return err
}
if err := store.EnsureSchema(ctx); err != nil { // or apply the DDL via migrations
	return err
}
```

Persistent storage on PostgreSQL 12+ (`DialectPostgres`) or MySQL 8.0+ / MariaDB 10.6+ (`DialectMySQL`) through `database/sql`. The application
owns the `*sql.DB` and chooses the driver.

```yaml
storage:
  type: sql
  sql:
    dialect: postgres                # Default: postgres
    tasksTable: scheduler_tasks      # Default: scheduler_tasks
    historyTable: scheduler_history  # Default: scheduler_history
    ensureSchema: false              # Default: false — run EnsureSchema during Build
```

**Characteristics:**
- `UpsertTask`, `CreateTask`, `ClaimRun`, `RenewRun`, `FinishRun` and `ReplaceTaskIf` are each a single statement (conditional where the run
  ownership rules fence it) that sets the revision; the row lock arbitrates concurrent schedulers
- String columns compare exactly (no trailing-space padding, no case folding): PostgreSQL IDs use `COLLATE "C"`, MySQL/MariaDB string columns are
  binary types (`VARBINARY`, `MEDIUMBLOB`, `LONGBLOB`) — no server version probe, and the schema never inherits database defaults. Tables created
  by the previous release (`utf8mb4` with a NO PAD binary collation) are exact too and keep working unchanged
- Filter expressions are translated to SQL by the `data/filter` PostgreSQL and MariaDB translators; `size()` counts characters, as on every
  backend. On MySQL/MariaDB string fields are filtered through a `utf8mb4_bin` text view, which is `PAD SPACE`: filter comparisons and `in`
  ignore trailing spaces there, while `contains()`, `startsWith()`, `endsWith()` and `size()` stay exact (lookups and run-ownership fences
  stay exact too)
- `DeleteTask` removes the task and its history in one transaction
- `EnsureSchema` is idempotent; the factory runs it only with `ensureSchema: true` — otherwise call it on a `sqldb.New` storage built from the
  same handle, dialect and table names (or migrate) before starting the scheduler
- `EnsureSchema` also adds the run-lease and occurrence columns (`run_lease_until`, `run_lease_id`, `run_at`) to a tasks table created by an
  earlier release; run it (or add them through your migrations) before upgrading the scheduler

See the package [README](../../service/scheduler/storages/sqldb/README.md) for the schema and dialect details.

---

## Distributed scheduling

In multi-node deployments, use a leader elector so only one instance dispatches tasks. All instances persist state, but only the leader executes task
functions.

```go
sched := scheduler.New(store,
	scheduler.WithLeaderElector(elector),
)

if sched.IsLeader() {
	// This instance is dispatching tasks.
}
```

When no `WithLeaderElector` is provided, `IsLeader` always returns `true`.

### Single execution: what the storage layer enforces

Leadership is a **throughput optimization, not a correctness dependency**. A leader-election lease can briefly overlap — a frozen or partitioned
former leader may still believe `IsLeader()` while a new leader takes over — so two instances can dispatch the *same* occurrence at once. To prevent
competing claims regardless, the scheduler claims each run through `Storage.ClaimRun`, a single atomic compare-and-swap:

- The write matches on `status == active` and, for the scheduled occurrence, `next_run_at == expectedNextRunAt` (the occurrence fence), flipping the
  task to `running` in one operation. Exactly one concurrent caller can match, so exactly one wins the claim; the loser skips the tick.
- MongoDB implements it as one conditional `UpdateOne`; Redis as a single server-side `EVAL` (Lua) script; the SQL backend as one conditional
  `UPDATE`; the memory backend under its mutex. All four are atomic read-check-write, so the guarantee holds even during a leader-election
  split-brain window.

The result: even if `IsLeader()` is wrong for a moment, competing dispatchers cannot both claim an occurrence. A one-shot task's `run_at` is fenced
along with `next_run_at`, so a dispatch that observed one registered occurrence cannot claim another. The claim also requires that no earlier run of the
task is unfinished (`run_started_at == 0`), so pausing and resuming — or disabling and enabling — a task while its run is in flight does not let a
second instance start the next occurrence before the first run has recorded its result.

Registration cannot undo a claim either. `Register` creates a new task with `Storage.CreateTask`, an atomic insert-if-absent; an instance that
loses the creation race re-reads the task and merges its configuration with a fenced `ReplaceTaskIf`, preserving the run another instance may
already have claimed.

#### Run ownership and stale recovery

Every run ID names the instance that executes it (`<instance-id>/<random>`, see `WithInstanceID`), and every run carries a lease. `ClaimRun` stores the
first `run_lease_until = start + lease` atomically with the claim; then, every third of the lease, the owner calls `Storage.RenewRun`, which moves it to
`now + lease` while the run is still its own. Each renewal is issued a third of a lease after the previous lease write, leaving two thirds of a lease
for it to land; a claim that itself took a third of a lease or more renews and confirms ownership — with a renewal answered within a third of a lease —
before the task body starts, and the body does not start if the run was taken over meanwhile. The lease is `WithStaleTaskTimeout`, floored at five
seconds because leases are stored in whole seconds. Stale recovery — at startup and periodically — resets a run only when it is:

- owned by this instance and no longer executing here (a restart with a stable `WithInstanceID`, or a run whose result write failed), or
- owned by anyone else and its lease has expired: the current time is past the persisted `run_lease_until`. Expiry is judged against the owner's
  persisted value, so instances configured with different timeouts agree. A lease counts only while it is bound to the run: `ClaimRun` and
  `RenewRun` store the run ID in `run_lease_id` with the lease, and `FinishRun` and recovery clear both. A run claimed by a release without
  leases has no lease of its own — `run_lease_until` zero, or an earlier run's lease still bound to that run — and falls back to its start plus
  the local lease. A lease stored without `run_lease_id` (written by a lease-aware build that did not bind leases yet) may belong to this run or an
  earlier one, so the later of both deadlines applies.

An instance runs a task only for the occurrence it registered: a one-shot task whose stored `RunAt` (or kind) differs from the local registration —
re-registered locally or elsewhere — is skipped rather than run with a stale function. Under these rules a run that is still executing on a live
instance is never reset, however long it takes, and a one-shot task is not re-executed while its first run is alive. Recovery of a task that management
paused, disabled or resumed during the run clears the run and keeps that status. With the default random instance ID, a crashed instance's runs are
recovered once their lease expires rather than immediately; give each instance a stable, unique `WithInstanceID` (for example the pod name) to recover
its own runs at once after a restart.

What remains: the lease relies on the owner being able to reach the storage and on roughly synchronized clocks. An owner that cannot renew for
longer than its lease (a partition, a stalled process) can be recovered while it still executes, so task effects that must not repeat should
still be idempotent or fenced externally, e.g. with the fencing token exposed by [`data/leadelect`](../../data/leadelect/README.md)
(`LeaderElector.Fence`). Instances running a release without run leases recover every `running` task at startup and do not renew leases; upgrade
all instances sharing a storage.

---

## Readiness probe

Defer task dispatch until all subsystems (databases, caches, message brokers) have finished initializing. The probe is evaluated at the start of each
tick — when it returns `false`, the tick is skipped but the main loop keeps running so `Stop` works cleanly.

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

`TasksPaginated` and `HistoryPaginated` accept a filter expression and cursor-based pagination. Filters are pushed down to the storage layer for
server-side evaluation (MongoDB, SQL), evaluated client-side (memory), or both — numeric ranges on the server, the whole expression on the client
(Redis).

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

- **Tasks**: `id`, `description`, `status`, `priority`, `schedule`, `lastRunAt`, `nextRunAt`, `skipNextRun`,
  `disableHistory`, `unmanaged`, `oneShot`, `failures`
- **History**: `id`, `taskId`, `runId`, `startedAt`, `endedAt`, `durationMs`, `success`, `error`

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

When a `metrics.Collector` is provided via `WithCollector`, the scheduler records Prometheus metrics under the `scheduler` subsystem. When no
collector is configured, a no-op implementation is used and all operations are zero-cost.

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

| Error | Returned by | Cause |
|---|---|---|
| `ErrTaskNotFound` | `PauseTask`, `ResumeTask`, `DisableTask`, `EnableTask`, `SkipNextRun`, `TriggerTask` | No task with the given ID in storage |
| `ErrTaskNotRegistered` | `Unregister`, `TriggerTask` | Task ID not in the in-memory registration map |
| `ErrTaskUnmanaged` | `PauseTask`, `DisableTask` | Task has the `Unmanaged` flag set |
| `ErrTaskNotPaused` | `ResumeTask` | Task status is not `Paused` |
| `ErrTaskNotDisabled` | `EnableTask` | Task status is not `Disabled` |
| `ErrTaskDisabled` | `TriggerTask` | Task status is `Disabled` |
| `ErrTaskCompleted` | `PauseTask`, `ResumeTask`, `DisableTask`, `EnableTask`, `TriggerTask` | One-shot task already executed |
| `ErrNotReady` | `TriggerTask` | Readiness probe returned `false` |
| `ErrScheduleConflict` | `Register` | Both `RunAt` and `Schedule` provided |
| `ErrInvalidCursor` | `TasksPaginated`, `HistoryPaginated` | Cursor malformed or filter changed since issue |
| `ErrInvalidRunClaim` | `Storage.ClaimRun` | Claim with a non-positive start, an empty run ID or a negative lease (`RunClaim.Validate`) |

---

## API reference

### Types

| Type             | Description                                                                    |
|------------------|--------------------------------------------------------------------------------|
| `Scheduler`      | Core scheduler: register, dispatch, pause/resume/disable, query                |
| `Storage`        | Persistence interface (15 methods) implemented by every backend                |
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
| `WithStaleTaskTimeout`          | `30m`           | Run lease (min `5s`); runs whose lease expired are recovered    |
| `WithInstanceID`                | random          | Owner ID stamped on runs; unique per live instance              |
| `WithLeaderElector`             | `nil`           | Distributed leader elector -- only the leader dispatches        |
| `WithReadinessProbe`            | `nil`           | Pre-dispatch readiness check -- skips tick when `false`         |
| `WithLogger`                    | discard         | Structured logger for lifecycle and error events                |
| `WithCollector`                 | noop            | Metrics collector for Prometheus instrumentation                |

---

## Storage backends

| Backend  | Notes                                                |
|----------|------------------------------------------------------|
| Memory   | In-process, deep-copy semantics. Tests, single node. |
| MongoDB  | Indexed queries, durable across restarts.            |
| Redis    | RedisJSON + RediSearch, durable, low-latency.        |

A configuration-driven builder selects the backend from YAML and wires logger, leader elector, metrics collector, and readiness probe.
