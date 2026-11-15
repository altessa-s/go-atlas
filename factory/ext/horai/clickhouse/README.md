# factory

```go
import chfactory "github.com/altessa-s/go-atlas/factory/ext/horai/clickhouse"
```

Maps the `historyStorage.clickhouse` block of the scheduler configuration to a [ClickHouse history storage][storage] for
[`factory/ext/horai`](..), which builds no history storage of its own. A zero `ttl` takes `historyRetention`; with
`ensureSchema` the table is created at startup (bounded to 30 s), otherwise it must exist (`EnsureSchema` on the storage, or `SchemaDDL` through
migrations).

## Usage

```go
sched, err := schedulerfactory.New(&cfg.Scheduler).
    UseHistoryStorage(schedulerconfig.HistoryStorageTypeClickHouse, chfactory.Builder(conn)).
    Build(ctx)
```

| Symbol            | Description                                                                                                               |
|-------------------|---------------------------------------------------------------------------------------------------------------------------|
| `Builder(conn)`   | History storage builder for the `clickhouse` type; the caller owns `conn` (e.g. from `factory/infrastructure/clickhouse`) |
| `ErrConnRequired` | Returned when the builder has no connection                                                                               |

[storage]: https://github.com/altessa-s/go-horai/tree/main/storage/clickhouse
