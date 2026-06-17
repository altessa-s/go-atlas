# saga

```go
import "github.com/altessa-s/go-atlas/data/saga"
```

Package `saga` runs inter-service distributed transactions with the orchestration-based saga pattern: a sequence of local steps, each with an optional
compensating action. When a step fails, the already-completed steps are rolled back in reverse order, so the overall operation leaves no partial effects.
The orchestrator is generic over the saga's shared data type `T` and persists a checkpoint to a pluggable [`Store`](./store.go) after every stage, so an
instance survives a crash and can be resumed or automatically rolled back.

## Model

| Concept           | Description                                                                                                     |
|-------------------|---------------------------------------------------------------------------------------------------------------|
| `Definition[T]`   | An ordered list of stages, built with the fluent `Builder`. A stage is one step or a parallel group of steps.  |
| Step              | A forward action plus an optional `Compensate`. A step with no compensation is treated as read-only.           |
| Pivot             | The point of no return: failures at or after the pivot stage roll forward (retry) instead of compensating.     |
| `Orchestrator[T]` | Drives a definition over a `Store`; `Start` runs a new instance, `Resume` continues a persisted one.           |
| `Instance`        | The persisted state of one execution: status, stage cursor, serialized data, version, deadline.                |

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
| `WithScheduler`               | none           | Registers the recovery cycle with a `core/scheduler.TaskRegistrar`.               |
| `WithRecoverySchedule`        | `@every 1m`    | Cron expression for the scheduled recovery cycle.                                 |
| `WithRecoveryBatchSize`       | 100            | Max instances processed per recovery cycle.                                       |
| `WithLeaderElector`           | none           | Gates the recovery cycle to the elected leader (e.g. a `*leadelect.Leader`).      |
| `WithCollector`               | no-op          | Prometheus metrics collector.                                                     |
| `WithSerializer`              | JSON           | Codec for the saga data persisted in `Instance.Data`.                             |
| `WithOnDeadLetter`            | none           | Hook fired when an instance reaches the terminal `Failed` state.                  |
| `WithShouldRetry`             | retry all      | Predicate deciding whether a step/compensation error is retryable.               |
| `WithCompensationPolicy`      | `PolicyWarn`   | Build-time check (`Warn`/`Enforce`/`Disabled`) for compensatable steps missing a compensation. |

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
// err != nil ⇒ inst.Status is StatusCompensated (rolled back) or StatusFailed.
```

Step actions and compensations must be **idempotent**: on interruption a step may run again when the instance is resumed. Steps inside a parallel group run
concurrently and share `*T`, so they must not write overlapping fields.

## Subpackages

| Package                                | Description                                                     |
|----------------------------------------|----------------------------------------------------------------|
| [errs](./errs)                         | Sentinel errors returned by the orchestrator and stores.       |
| [factory](./factory)                   | Config-driven assembly of an `Orchestrator` from `config.Saga`.|
| [storages/memory](./storages/memory)   | In-process `Store` backend (reference implementation).         |
| [storages/mongo](./storages/mongo)     | Durable `Store` backend on a MongoDB collection.               |
| [storages/nats](./storages/nats)       | Durable `Store` backend on NATS JetStream KeyValue.            |
| [storages/redis](./storages/redis)     | Durable `Store` backend on Redis (hash + sorted-set index).    |
