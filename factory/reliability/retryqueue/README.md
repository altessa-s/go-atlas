# factory

```go
import retryqueuefactory "github.com/altessa-s/go-atlas/factory/reliability/retryqueue"
```

Assembles a [`retryqueue.Storage`](../../../reliability/retryqueue/storage.go) from a
[`retryqueueconfig.Config`](../../../config/retryqueue/retryqueue.go) and injected backend clients. The builder is generic over the payload type `P`, so
`Build` returns a fully typed `retryqueue.Storage[P]`; every bundled store also implements `retryqueue.Mover`.

## Builder

| Method        | Description                                                                     |
|---------------|---------------------------------------------------------------------------------|
| `New[P](cfg)` | Create a builder for a config (validated at `Build`).                           |
| `Build(ctx)`  | Validate config and construct the store; `ctx` bounds schema or index creation. |

Register a custom backend name with `UseStorage`. Before standalone validation or config loading, call
`cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())`.

## Injected dependencies

The backend is chosen by `retryqueueconfig.Config.Storage.Type`. The builder builds only `memory` and `sqldb`; `mongo` uses the function registered with
`UseStorage` from [`storages/mongo`](storages/mongo), and without one `Build` fails with `ErrNotRegistered` naming the package to add.

| Method             | Required for `storage.type` | Description                                                                         |
|--------------------|-----------------------------|-------------------------------------------------------------------------------------|
| `UseStorage`       | `mongo`                     | Registers the `StorageFunc[P]` of a type the builder does not build.                |
| `UseSQLDB`         | `sqldb`                     | `*sql.DB`; caller registers the driver.                                             |
| `UseSQLPool`       | `sqldb`                     | Supplies the pool and per-call transaction resolver.                                |
| `UseCodec`         | —                           | Payload codec of the sqldb store (default JSON).                                    |
| `UseLogger`        | —                           | Logger for the builder and the sqldb background purge.                              |
| `UseShutdownHooks` | —                           | Hook group that receives the sqldb store's `StopPurge`; default: process-wide registry. |
| `UseDefaultLogger` | —                           | Use `slog.Default()` as the logger.                                                 |

With `storage.sqldb.resolvedRetention` set, the sqldb store purges resolved items in one of two ways: `storage.sqldb.purgeInterval` runs `PurgeExpired`
in the background from the first write call, and the shutdown hook calls `StopPurge` to join the worker. With `purgeInterval: "0s"` the
application asserts `PurgeExpired(ctx) (int64, error)` on the store and calls it itself, e.g. from a scheduler. The MongoDB store purges through a TTL
index instead. `storage.sqldb.purgeBatchSize` (default 1000) bounds one purge statement. Run shutdown hooks before closing the injected database.

## Usage

```go
cfg := retryqueueconfig.Default()
cfg.Storage = &retryqueueconfig.StorageConfig{
	Type: retryqueueconfig.StorageTypeSQL,
	SQL:  &retryqueueconfig.SQLStorageConfig{Dialect: "postgres", Table: "retry_items", EnsureSchema: true},
}

queue, err := retryqueuefactory.New[Valuation](&cfg).
	UseSQLDB(db).
	Build(ctx)
```

## See also

- [retryqueueconfig.Config](../../../config/retryqueue/retryqueue.go) — the configuration this factory consumes.
- [storages](../../../reliability/retryqueue/storages) — the backends it selects between.
