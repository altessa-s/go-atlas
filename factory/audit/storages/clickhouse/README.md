# factory

```go
import auditchfactory "github.com/altessa-s/go-atlas/factory/audit/storages/clickhouse"
```

Maps the `storage.clickhouse` block of the audit configuration to a [ClickHouse audit storage](../../../../audit/storages/clickhouse/) for
[`factory/audit`](../..), which builds only the memory, mongo and sqldb storages. The table is checked against the expected schema at startup,
bounded by `ddlTimeout`.

## Usage

```go
auditor, err := auditfactory.New(&cfg.Audit).
    UseStorage(auditconfig.StorageTypeClickHouse, auditchfactory.Builder(conn)).
    Build(ctx)
```

| Symbol                    | Description                                                                                                       |
|---------------------------|-------------------------------------------------------------------------------------------------------------------|
| `Builder(conn)`           | Storage builder for the `clickhouse` type; the caller owns `conn` (e.g. from `factory/infrastructure/clickhouse`) |
| `ErrConnRequired`         | Returned when the builder has no connection                                                                       |
| `MinRecommendedBatchSize` | A dispatch `batchSize` below it logs a warning: every flush creates a part to merge                               |
