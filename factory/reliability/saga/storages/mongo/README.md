# factory

```go
import mongofactory "github.com/altessa-s/go-atlas/factory/reliability/saga/storages/mongo"
```

Maps the `mongo` block of the saga storage configuration to a [MongoDB saga store](../../../../../reliability/saga/storages/mongo/) for
[`factory/reliability/saga`](../..), which builds only the memory and sqldb stores.

## Usage

```go
orch, err := sagafactory.New(&cfg, def).
    UseStorage(sagaconfig.StorageTypeMongo, mongofactory.Builder(db)).
    Build(ctx)
```

| Symbol              | Description                                                                                                      |
|---------------------|------------------------------------------------------------------------------------------------------------------|
| `Builder(db)`       | Storage function for the `mongo` type; instances live in `storage.mongo.collection`, `ctx` bounds index creation |
| `ErrClientRequired` | Returned when the function has no database                                                                       |
| `ErrConfigRequired` | Returned when the configuration has no `storage.mongo` block                                                     |
