# factory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/factory"
```

Package `factory` builds `scheduler.Scheduler` instances and their `scheduler.Storage` backends from configuration
objects. Infrastructure references (MongoDB database, Redis client, leader elector) are injected once at construction
and reused across every scheduler the factory creates. Call `CreateStorageFromConfig` to obtain a storage backend,
then `CreateSchedulerFromConfig` to wire up the scheduler with all options derived from configuration.

## Usage

```go
f := factory.New(
    factory.WithLogger(logger),
    factory.WithMongoDb(db),
    factory.WithLeaderElector(le),
)

storage, err := f.CreateStorageFromConfig(cfg.Storage)
if err != nil {
    return err
}

sched, err := f.CreateSchedulerFromConfig(cfg, storage)
if err != nil {
    return err
}
```

## Supported storage backends

| Config value | Backend   | Requirement                                                                   |
|--------------|-----------|-------------------------------------------------------------------------------|
| `memory`     | In-memory | None — used by default, suitable for single-instance and testing              |
| `mongodb`    | MongoDB   | `WithMongoDb(db)` must be called at factory construction time                 |
| `redis`      | Redis     | `WithRedisClient(client)` must be called at factory construction time         |
