# factory

```go
import sagafactory "github.com/altessa-s/go-atlas/data/saga/factory"
```

Assembles a [`saga.Orchestrator`](../orchestrator.go) from a [`config.Saga`](../../../config/saga.go) and injected backend clients. The builder is
generic over the saga's shared data type `T`, so `Build` returns a fully typed `*saga.Orchestrator[T]`.

## Builder

| Method          | Description                                                                |
|-----------------|----------------------------------------------------------------------------|
| `New(cfg, def)` | Create a builder for a config and definition (both validated at `Build`).  |
| `Build()`       | Validate config, construct the store, return the orchestrator.             |

## Injected dependencies

The store backend is chosen by `config.Saga.Storage.Type`; the matching client must be injected or `Build` fails with `<dependency> is required`.

| Method             | Required for `storage.type` | Description                          |
|--------------------|-----------------------------|--------------------------------------|
| `UseJetStream`     | `nats`                      | NATS JetStream context.              |
| `UseMongoDatabase` | `mongo`                     | MongoDB database handle.             |
| `UseRedisClient`   | `redis`                     | Redis client.                        |
| `UseLogger`        | —                           | Logger for builder and orchestrator. |
| `UseCollector`     | —                           | Prometheus metrics collector.        |
| `UseScheduler`     | —                           | Registrar for the recovery cycle.    |
| `UseLeaderElector` | —                           | Gate recovery to the elected leader. |
| `UseSerializer`    | —                           | Payload codec (default JSON).        |
| `UseOnDeadLetter`  | —                           | Hook for terminal `Failed` instances. |
| `UseShouldRetry`   | —                           | Predicate deciding retryable errors. |

`memory` requires no client.

## Usage

```go
def := saga.NewDefinition[Order]("place-order").
	Step("reserve", reserve).Compensate(release).
	Step("charge", charge).Compensate(refund).Pivot().
	MustBuild()

cfg := config.DefaultSaga()
cfg.Storage = &config.SagaStorageConfig{
	Type:  config.SagaStorageTypeMongo,
	Mongo: &config.SagaMongoStorageConfig{Collection: "saga_instances"},
}

orch, err := factory.New(&cfg, def).
	UseMongoDatabase(db).
	UseScheduler(scheduler).
	Build()
```

## See also

- [config.Saga](../../../config/saga.go) — the configuration this factory consumes.
- [storages](../storages) — the backends it selects between.
