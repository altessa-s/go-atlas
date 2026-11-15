# bloom

```go
import "github.com/altessa-s/go-atlas/cache/probfilter/bloom"
```

Package `bloom` provides a Bloom filter implementation for probabilistic existence checks. Bloom filters are space-efficient but do not support
deletion. Use periodic rebuilds via `RebuildableFilter` when the underlying dataset changes.

`Rebuild` is atomic: the replacement is built off to the side (a fresh in-process filter, or a Redis staging key renamed onto the live key) while
lookups keep seeing the previous contents; a failed or canceled rebuild leaves them unchanged (after `probfilter.ErrCommitIndeterminate` either the
previous or the rebuilt contents are in place), and values added through the filter during the rebuild are replayed onto the replacement. A shared
Redis filter journals the adds of other processes made under the rebuild lease in Redis and replays them in the commit, so they survive as well.

## Rebuild lifecycle

`New` has no side effects. `StartRebuild(ctx)` starts the optional lifecycle configured by the options below: with a loader it rebuilds the
filter synchronously (`WithRebuildOnStart`) and runs the periodic rebuild where the contents live. A process-local storage gets a process-local
cron that skips ticks while a rebuild runs, logs failed rebuilds and stops when the filter is closed; a shared storage (Redis) is registered once
as task `probfilter-rebuild-<name>` with the scheduler, whose task reports rebuild errors and is a no-op once the filter is closed or while another
process rebuilds it (without a scheduler the schedule is logged as ignored). A failed `StartRebuild` leaves the filter open for the caller to
close. A filter that never calls it keeps `Rebuild` and `Done` for an externally owned schedule.

## Options

| Option                          | Default | Description                                                                  |
|---------------------------------|---------|------------------------------------------------------------------------------|
| `WithName`                      | `bloom` | Filter name in the rebuild task ID and in log records                        |
| `WithLogger`                    | discard | Logger for the rebuild lifecycle                                             |
| `WithRebuildLoader`             | none    | Data source of the lifecycle; without it `StartRebuild` is inert             |
| `WithRebuildOnStart`            | off     | Rebuild synchronously inside `StartRebuild`                                  |
| `WithRebuildSchedule`           | none    | Cron expression of periodic rebuilds                                         |
| `WithScheduler`                 | none    | `schedtask.Registrar` that runs the periodic rebuild of a shared storage     |
| `WithTolerateRebuildInProgress` | off     | Let the initial rebuild of a shared filter lose the lease to another process |

| Error                       | Returned when                                     |
|-----------------------------|---------------------------------------------------|
| `ErrInvalidRebuildSchedule` | The local cron schedule does not parse            |
| `ErrRebuildAlreadyStarted`  | `StartRebuild` is called again after it succeeded |

## Key types

| Type     | Description                                                                                                               |
|----------|---------------------------------------------------------------------------------------------------------------------------|
| `Filter` | Bloom filter — implements `Filter`, `RebuildableFilter`, `ObservableFilter`, `RebuildCommitReporter`, and `StatsProvider` |

## Subpackages

| Package                              | Description          |
|--------------------------------------|----------------------|
| [storages](./storages)               | Storage interface    |
| [storages/memory](./storages/memory) | In-memory backend    |
| [storages/redis](./storages/redis)   | Redis-backed backend |
