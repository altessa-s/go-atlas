# factory

```go
import auditmongofactory "github.com/altessa-s/go-atlas/factory/audit/storages/mongo"
```

Maps the `storage.mongo` block of the audit configuration to a [MongoDB audit storage](../../../../audit/storages/mongo/) for
[`factory/audit`](../..), which builds only the memory and sqldb storages. The block tunes collection name, index timeout and TTL; omitting it
means all defaults. The indexes are created at startup on the build context.

## Usage

```go
auditor, err := auditfactory.New(&cfg.Audit).
    UseStorage(auditconfig.StorageTypeMongo, auditmongofactory.Builder(db)).
    Build(ctx)
```

| Symbol                | Description                                                |
|-----------------------|------------------------------------------------------------|
| `Builder(db)`         | Storage builder for the `mongo` type; the caller owns `db` |
| `ErrDatabaseRequired` | Returned when the builder has no database                  |
