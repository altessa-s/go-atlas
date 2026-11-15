# factory

```go
import mongofactory "github.com/altessa-s/go-atlas/factory/ext/horai/mongo"
```

Maps the `storage.mongo` block of the scheduler configuration to a [MongoDB task storage][storage] for
[`factory/ext/horai`](..), which builds only the memory and sqldb task storages. With `ensureIndexes` the indexes are created
at startup (bounded to 30 s); otherwise the builder performs no I/O and the indexes must exist (`EnsureIndexes` on the storage).

## Usage

```go
sched, err := schedulerfactory.New(&cfg.Scheduler).
    UseStorage(schedulerconfig.StorageTypeMongo, mongofactory.Builder(db)).
    Build(ctx)
```

| Symbol                | Description                                                      |
|-----------------------|------------------------------------------------------------------|
| `Builder(db)`         | Task storage function for the `mongo` type; the caller owns `db` |
| `ErrDatabaseRequired` | Returned when the function has no database                       |
| `ErrConfigRequired`   | Returned when the configuration has no `mongo` block             |

[storage]: https://github.com/altessa-s/go-horai/tree/main/storage/mongo
