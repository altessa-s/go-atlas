# idempotencyconfig

```go
import idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
```

Package `idempotencyconfig` defines the idempotency key storage schema. Schemas are populated by [`go-nomos/loader`](https://github.com/altessa-s/go-
nomos/tree/develop/loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                 | Description                                                                  |
|------------------------|--------------------------------------------------------------------------------|
| `Config`             | Defines the configuration for idempotency key management.                    |
| `StorageType`        | Selects the backend that stores idempotency keys.                            |
| `StorageConfig`      | Selects the key store: `memory`, `redis`, `nats`, `mongo` or `sqldb`            |
| `MongoStorageConfig` | MongoDB collection for the keys; they commit with the caller's transaction   |
| `SQLStorageConfig`   | SQL dialect, table, cleanup interval and optional startup schema creation                     |

See the [config index](../README.md) for the other schema packages.

## SQL storage

`StorageTypeSQL` selects PostgreSQL (`postgres`) or MySQL/MariaDB (`mysql`). `SQLStorageConfig` maps to `sqldb: {dialect, table, ensureSchema,
purgeInterval, purgeBatchSize}`. Defaults are `postgres`, `idempotency_keys`, `false` and `1m` (zero disables cleanup). Inject `*sql.DB` with the
[factory](../../factory/reliability/idempotency/README.md)'s `UseSQLDB`.

```yaml
idempotency:
  ttl: 24h
  maxLockDuration: 5m
  storage:
    type: sqldb
    sqldb:
      dialect: postgres
      table: idempotency_keys
      ensureSchema: false
      purgeInterval: "1m"
      purgeBatchSize: 1000
```

Use migrations when `ensureSchema` is false. SQL expiry uses the database clock; the lazy background sweep removes expired rows. The factory registers
cleanup shutdown hooks; use `UseShutdownHooks(group)` for a component scope and run hooks before closing the injected database. See [the storage
template](../templates/idempotency_storage.yaml).

The sqldb block exposes purgeInterval (default "1m", "0s" disables the background loop) and purgeBatchSize (default 1000). Manual PurgeExpired remains
available with a zero interval. Purging runs only when retention is enabled; idempotency keys always have their own expiry.
