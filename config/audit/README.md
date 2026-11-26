# auditconfig

```go
import auditconfig "github.com/altessa-s/go-atlas/config/audit"
```

Package `auditconfig` defines the audit event dispatcher schema. Schemas are populated by [`go-nomos/loader`](https://github.com/altessa-s/go-
nomos/tree/develop/loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                | Description                                                   |
|-----------------------|------------------------------------------------------------------|
| `StorageType`       | Defines the type of storage backend for audit events.            |
| `Config`            | Defines the configuration for the audit subsystem.               |
| `Storage`           | Defines the storage backend configuration for audit events.      |
| `StorageMemory`     | Holds bounded ring-buffer settings for in-memory audit storage.  |
| `StorageMongo`      | Holds MongoDB-specific configuration for audit event storage.    |
| `StorageSQL`        | Holds SQL-specific configuration for audit event storage.        |
| `StorageClickHouse` | Holds ClickHouse-specific configuration for audit event storage. |

Omitting `storage.type` falls back to the bounded in-memory ring buffer (`memory`, default `maxEvents: 10000`, oldest evicted) and causes
[`factory/audit`](../../factory/audit) to log a startup warning. Explicit `storage.type: memory` selects the same bounded ring buffer without the
default-fallback warning.

The `paging` section of event queries is the shared [`config/paging`](../paging) schema (nil disables page tokens).

`storage.sqldb.ttl` uses the same duration field and YAML key as `storage.mongo.ttl`: zero keeps events forever; nonpositive values leave retention
disabled. Because MongoDB's TTL index stores a 32-bit `expireAfterSeconds`, `storage.mongo.ttl` rejects sub-second positive values (`0 < ttl < 1s`) and
values exceeding `math.MaxInt32` seconds. SQL retention runs in the storage on `purgeInterval` (default `"1m"`) in batches of up to `purgeBatchSize`
(default 1000), while MongoDB uses a server TTL index. The SQL factory maps `ttl`, `purgeInterval` and `purgeBatchSize` to the storage options.

See the [config index](../README.md) for the other schema packages.

The sqldb block exposes `purgeInterval` (default `"1m"`, `"0s"` disables the background loop) and `purgeBatchSize` (default 1000). Manual `PurgeExpired`
remains available with a zero interval. Purging runs only when retention (`ttl`) is enabled.
