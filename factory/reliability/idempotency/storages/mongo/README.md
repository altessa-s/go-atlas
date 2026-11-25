# factory

```go
import mongofactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency/storages/mongo"
```

Maps the `mongo` block of the idempotency storage configuration to a [MongoDB idempotency storage](../../../../../reliability/idempotency/storages/mongo/) for
[`factory/reliability/idempotency`](../..), which builds only the in-memory storage.

## Usage

```go
keeper, err := idempotencyfactory.New(cfg.Idempotency).
    UseStorage(idempotencyconfig.StorageTypeMongo, mongofactory.Builder(db)).
    Build(ctx)
```

| Symbol              | Description                                                                                           |
|---------------------|-------------------------------------------------------------------------------------------------------|
| `Builder(db)`       | Storage function for the `mongo` type; keys live in `storage.mongo.collection` and expire after `ttl` |
| `ErrClientRequired` | Returned when the function has no database                                                            |
| `ErrConfigRequired` | Returned when the configuration has no `storage.mongo` block                                          |
