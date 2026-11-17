# factory

```go
import mongofactory "github.com/altessa-s/go-atlas/factory/reliability/retryqueue/storages/mongo"
```

Maps the `mongo` block of the retry queue storage configuration to a [MongoDB retry queue store](../../../../../reliability/retryqueue/storages/mongo/)
for [`factory/reliability/retryqueue`](../..), which builds only the memory and sqldb stores.

## Usage

```go
queue, err := retryqueuefactory.New[Valuation](&cfg).
    UseStorage(retryqueueconfig.StorageTypeMongo, mongofactory.Builder[Valuation](db)).
    Build(ctx)
```

| Symbol              | Description                                                                                                          |
|---------------------|----------------------------------------------------------------------------------------------------------------------|
| `Builder[P](db)`    | Storage function for the `mongo` type: `storage.mongo.collection` of `db`, flag fields and resolved retention as set |
| `ErrClientRequired` | Returned when the function has no database; wraps `factory/core.ErrDependencyRequired`                               |
| `ErrConfigRequired` | Returned when the configuration has no `storage.mongo` block; wraps `factory/core.ErrConfigRequired`                 |

With `storage.mongo.ensureIndexes` set, the function creates the store's indexes (bounded by the build context); otherwise it performs no I/O.
