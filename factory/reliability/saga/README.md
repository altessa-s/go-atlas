# factory

```go
import sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
```

Assembles a [`saga.Orchestrator`](../../../reliability/saga/orchestrator.go) from a [`sagaconfig.Config`](../../../config/saga/saga.go) and injected backend
clients. The builder is generic over the saga's shared data type `T`, so `Build` returns a fully typed `*saga.Orchestrator[T]`.

## Builder

| Method          | Description                                                                                                        |
|-----------------|--------------------------------------------------------------------------------------------------------------------|
| `New(cfg, def)` | Create a builder for a config and definition (both validated at `Build`).                                          |
| `Build(ctx)`    | Validate config, construct the store, register the recovery cycle, return the orchestrator; `ctx` bounds that I/O. |

## Injected dependencies

The store backend is chosen by `sagaconfig.Config.Storage.Type`. The builder builds only `memory` and `sqldb`; `nats`, `mongo` and `redis` use
the function registered with `UseStorage` from `storages/<backend>/factory`, and without one `Build` fails naming the package to add.

| Method             | Required for `storage.type` | Description                                                             |
|--------------------|-----------------------------|-------------------------------------------------------------------------|
| `UseStorage`       | `nats`, `mongo`, `redis`    | Registers the `StorageFunc` of a store type the builder does not build. |
| `UseSQLDB`         | `sqldb`                     | `*sql.DB`; caller registers driver.                                     |
| `UseLogger`        | —                           | Logger for builder and orchestrator.                                    |
| `UseDefaultLogger` | —                           | Use `slog.Default()` as the logger.                                     |
| `UseMeter`         | —                           | Prometheus metrics collector.                                           |
| `UseScheduler`     | —                           | Registrar for the recovery cycle.                                       |
| `UseLeaderElector` | —                           | Gate recovery to the elected leader.                                    |
| `UseSerializer`    | —                           | Payload codec (default JSON).                                           |
| `UseOnDeadLetter`  | —                           | Hook for terminal `Failed` instances.                                   |
| `UseShouldRetry`   | —                           | Predicate deciding retryable errors.                                    |

| Type    | Register                                                                                                      |
|---------|---------------------------------------------------------------------------------------------------------------|
| `nats`  | `UseStorage("nats", natsfactory.Builder(js))` from [`factory/reliability/saga/storages/nats`](storages/nats)         |
| `mongo` | `UseStorage("mongo", mongofactory.Builder(db))` from [`factory/reliability/saga/storages/mongo`](storages/mongo)     |
| `redis` | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/reliability/saga/storages/redis`](storages/redis) |

`memory` requires no client.

## Usage

```go
def := saga.NewDefinition[Order]("place-order").
	Step("reserve", reserve).Compensate(release).
	Step("charge", charge).Compensate(refund).Pivot().
	MustBuild()

cfg := sagaconfig.Default()
cfg.Storage = &sagaconfig.StorageConfig{
	Type:  sagaconfig.StorageTypeMongo,
	Mongo: &sagaconfig.MongoStorageConfig{Collection: "saga_instances"},
}

orch, err := sagafactory.New(&cfg, def).
	UseStorage(sagaconfig.StorageTypeMongo, mongofactory.Builder(db)).
	UseScheduler(scheduler).
	Build(ctx)
```

## See also

- [sagaconfig.Config](../../../config/saga/saga.go) — the configuration this factory consumes.
- [storages](../../../reliability/saga/storages) — the backends it selects between.
