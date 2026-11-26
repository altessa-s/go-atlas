# saga

```go
import "github.com/altessa-s/go-atlas/reliability/saga"
```

Package `saga` runs inter-service distributed transactions with the orchestration-based saga pattern: a sequence of local steps, each with an optional
compensating action. When a step before the pivot fails, the already-completed steps are compensated in reverse order; a compensation that exhausts
its retries leaves the saga `Failed` with the rollback incomplete. The orchestrator is generic over the saga's shared data type `T` and persists a
checkpoint to a pluggable [`Storage`](./storage.go) after every stage, so an instance survives a crash and can be resumed or automatically rolled back.

## Model

| Concept           | Description                                                                                                     |
|-------------------|---------------------------------------------------------------------------------------------------------------|
| `Definition[T]`   | An ordered list of stages, built with the fluent `Builder`. A stage is one step or a parallel group of steps.  |
| Step              | A forward action plus an optional `Compensate`. A step with no compensation is skipped on rollback; mark one with nothing to undo `ReadOnly()` so the compensation policy does not flag it. |
| Pivot             | The point of no return: failures at or after the pivot stage roll forward (retry) instead of compensating.     |
| `Orchestrator[T]` | Drives a definition over a `Storage`; `Start` runs a new instance, `Resume` continues a persisted one.           |
| `Instance`        | The persisted state of one execution: status, stage cursor, serialized data, version, deadline.                |

`Start` writes the execution lease in the same call that creates the instance, so a stored instance is always either owned or recoverable. Recovery
marks an instance it can never run (undecodable payload, cursor outside the definition) `Failed` with a version-checked write and fires the dead-letter
hook. A step that started but ended with a context error (canceled sibling, step timeout) has an unknown outcome and is compensated. Terminal instances
stay until `WithRetention` is set; then `RunRetentionCycle` (or the scheduled task from `RegisterRetention`) deletes them via `Storage.DeleteTerminal`.

Quarantine is destructive: a failed instance keeps its committed stages without compensation. Change a definition or its payload type compatibly across
a rolling deploy, because a node on the old version quarantines instances the new one wrote once their lease expires. A stored record a backend cannot
decode at all (Redis, NATS) is reported by `FetchRecoverable` as a `Corrupt` stub and replaced by a `Failed` instance with a version-checked `Update`.
Retention deletes the instance and with it the idempotency record of its ID: a later `Start` with a retained-and-deleted ID runs the saga again. It needs
a named definition (`ErrEmptyDefinitionName`), and its task ID must differ from the recovery task ID (`ErrTaskIDCollision`); `RegisterTasks` installs both
all or nothing.

Storage implementations can return `ErrInvalidStorageKey` when they cannot represent an instance ID. Command consumers can then stop retrying that ID.

## Status lifecycle

`Running` → `Completed` (all stages committed) · `Running` → `Compensating` → `Compensated` (rolled back) · `Compensating`/post-pivot → `Failed`
(unrecoverable; needs intervention).

## Options

| Option                        | Default        | Description                                                                       |
|-------------------------------|----------------|-----------------------------------------------------------------------------------|
| `WithStepTimeout`             | 30s            | Per-step timeout for an action or compensation invocation.                        |
| `WithMaxStepAttempts`         | 3              | Total attempts (incl. first) for a forward step action.                           |
| `WithStepRetryBaseDelay`      | 100ms          | First exponential-backoff delay between step retries.                             |
| `WithStepRetryMaxDelay`       | 5s             | Cap on the step-retry backoff.                                                     |
| `WithMaxCompensationAttempts` | 5              | Total attempts for a single compensation before the saga fails.                   |
| `WithStepConcurrency`         | core default   | Fan-out limit for parallel-group steps (0 = IO-bound default).                    |
| `WithSagaTimeout`             | 0 (disabled)   | Per-instance deadline; enables auto-rollback by the recovery cycle.               |
| `WithExecutionTimeout`        | 5m             | Bounds one `Start` or `Resume` ownership period.                                  |
| `WithLeaseGrace`              | 30s            | Lets canceled operations finish before another owner takes over the lease.        |
| `WithStorageTimeout`          | 10s            | Bounds each persistence operation.                                                |
| `WithScheduler`               | none           | Registers the recovery cycle with a `schedtask.Registrar`.               |
| `WithRecoverySchedule`        | `@every 1m`    | Cron expression for the scheduled recovery cycle.                                 |
| `WithRecoveryBatchSize`       | 100            | Max instances processed per recovery cycle.                                       |
| `WithRecoveryTaskID`          | `saga-recovery-<definition>` | Scheduler task ID of the recovery cycle.                           |
| `WithRetention`               | 720h (30 days) | Keep terminal instances this long after their last update; the retention cycle (`RunRetentionCycle`, `RegisterRetention`) is on by default, `WithRetention(0)` disables it. The default applies to named definitions only. |
| `WithRetentionSchedule`       | `@every 1h`    | Cron expression for the scheduled retention cycle (task `saga-retention-<definition>`). |
| `WithRetentionBatchSize`      | 500            | Terminal instances one storage call deletes per retention batch.                  |
| `WithRecoveryTimeout`         | 5m             | Bounds an entire recovery pass.                                                   |
| `WithLeaderElector`           | none           | Gates the recovery cycle to the elected leader (e.g. a `*leadelect.Leader`).      |
| `WithMeter`                   | no-op          | Prometheus metrics collector.                                                     |
| `WithLogger`                  | discard        | Structured logger.                                                                |
| `WithContext`                 | `context.Background()` | Base context of the orchestrator.                                          |
| `WithSerializer`              | JSON           | Codec for the saga data persisted in `Instance.Data`.                             |
| `WithOnDeadLetter`            | none           | Hook fired when an instance reaches the terminal `Failed` state.                  |
| `WithShouldRetry`             | retry all      | Predicate deciding whether a step/compensation error is retryable.               |
| `WithCompensationPolicy`      | `PolicyWarn`   | `Build` option: check (`Warn`/`Enforce`/`Disabled`) for compensatable steps missing a compensation. |

## Usage

```go
def := saga.NewDefinition[Order]("place-order").
	Step("reserve-stock", reserveStock).Compensate(releaseStock).
	Parallel("notify",
		saga.NewStep("email", sendEmail).Compensate(unsend),
		saga.NewStep("audit", writeAudit).ReadOnly()).
	Step("charge-card", chargeCard).Compensate(refund).Pivot().
	Step("ship", ship).
	MustBuild()

orch := saga.New(memory.New(), def,
	saga.WithStepTimeout(10*time.Second),
	saga.WithSagaTimeout(5*time.Minute),
)

inst, err := orch.Start(ctx, order.ID, order)
// err != nil ⇒ inst may be nil (validation, create error); otherwise its Status is StatusCompensated (rolled back),
// StatusFailed, or still non-terminal when execution was interrupted (context or store error). An existing terminal
// instance (StatusCompleted included) comes back with ErrAlreadyTerminal.
```

Step actions and compensations must be **idempotent**: on interruption a step may run again when the instance is resumed. Steps inside a parallel group
run concurrently and share `*T`, so they must not write overlapping fields.

## Subpackages

| Package                                  | Description                                                       |
|------------------------------------------|-------------------------------------------------------------------|
| [engines/jetstream](./engines/jetstream) | JetStream work-queue engine: durable `Submit`, distributed `Run`. |
| [factory](../../factory/reliability/saga)                     | Config-driven assembly of an `Orchestrator` from `sagaconfig.Config`.   |
| [storages/memory](./storages/memory)     | In-process `Storage` backend (reference implementation).            |
| [storages/mongo](./storages/mongo)       | Durable `Storage` backend on a MongoDB collection.                  |
| [storages/nats](./storages/nats)         | Durable `Storage` backend on NATS JetStream KeyValue.               |
| [storages/redis](./storages/redis)       | Durable `Storage` backend on Redis (hash + sorted-set index).       |
| [storages/sqldb](./storages/sqldb)       | Durable `Storage` backend on PostgreSQL, MySQL or MariaDB.          |
| [storages/storagetest](./storages/storagetest) | Contract suite every `Storage` backend runs.            |

## Timeout composition

Execution, recovery, step and store timeouts are maximum budgets. Each nested operation gets the earlier of its caller's deadline and its own configured
deadline; an existing longer parent deadline never disables the local limit. This also applies to compensation and recovery persistence. Callbacks must
cooperate with context cancellation.
