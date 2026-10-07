# factory

```go
import chfactory "github.com/altessa-s/go-atlas/infrastructure/clickhouse/factory"
```

Builds a ClickHouse `driver.Conn` from [`clickhouseconfig.Config`](../../../config/clickhouse) using a fluent builder. The builder does
assembly only: it turns configuration and injected dependencies into a connection and hands ownership to the caller, who must close it.
Storage packages such as [`data/audit/storages/clickhouse`](../../../data/audit/storages/clickhouse) take that connection as a parameter
rather than dialing one themselves.

## Builder methods

| Method                    | Description                                                                            |
|---------------------------|----------------------------------------------------------------------------------------|
| `New(cfg)`                | Creates the builder. A nil config surfaces as an error at `Build` time.                  |
| `UseLogger(l)`            | Logger for the builder and the health checker.                                           |
| `UseDefaultLogger()`      | Shorthand for `UseLogger(slog.Default())`.                                               |
| `UseTlsConfig(c)`         | TLS for the connection. Overrides TLS carried by a connection DSN.                       |
| `UseHealthCoordinator(c)` | Registers a health checker for the created connection.                                   |
| `UseHealthServiceName(n)` | Name to register the checker under. Defaults to `DefaultHealthServiceName`.              |
| `ClientOptions()`         | Renders the driver options without opening a connection. Useful in tests.                |
| `Build(ctx)`              | Opens the connection and registers the health checker.                                   |

## Configuration

`clickhouseconfig.Config` accepts either a DSN or individual fields:

| Field             | Default          | Description                                                                   |
|-------------------|------------------|-------------------------------------------------------------------------------|
| `connectionURI`   | —                | Full DSN. When set, replaces `hosts`, `database`, `username`, `password`.       |
| `hosts`           | `localhost:9000` | Servers as `host:port`; the client fails over between them.                     |
| `database`        | `default`        | Database holding the target tables.                                             |
| `username`        | —                | ClickHouse user; empty uses the server's `default` user.                         |
| `password`        | —                | Secret; never logged.                                                           |
| `dialTimeout`     | `5s`             | Bounds establishing a connection.                                               |
| `readTimeout`     | `30s`            | Bounds reading a query result.                                                  |
| `maxOpenConns`    | `10`             | Pool ceiling.                                                                   |
| `maxIdleConns`    | `5`              | Idle connections kept in the pool.                                              |
| `connMaxLifetime` | `1h`             | How long a pooled connection may be reused.                                     |
| `compression`     | `lz4`            | `none`, `lz4`, `zstd`, `gzip`, `deflate`, or `br`.                              |
| `settings`        | —                | Server-side query settings applied to every statement.                          |
| `tls`             | —                | Standard `TlsClient` section.                                                   |

Setting `connectionURI` together with `username` or `password` is a validation error: the DSN already carries them. A malformed DSN fails
with `ErrInvalidConnectionURI`, without quoting the DSN or its credentials.

## Usage

```go
conn, err := chfactory.New(cfg).
    UseDefaultLogger().
    UseHealthCoordinator(coordinator).
    Build(ctx)
if err != nil {
    return err
}
defer conn.Close()

storage, err := auditclickhouse.New(conn)
```

```yaml
clickhouse:
  hosts:
    - clickhouse-1:9000
    - clickhouse-2:9000
  database: analytics
  username: writer
  password: ${CLICKHOUSE_PASSWORD}
  compression: lz4
  settings:
    max_execution_time: "60"
```

## Behavior worth knowing

- **The driver connects lazily.** A successful `Build` does not prove the server is reachable. Registering a health coordinator is what
  turns an unreachable server into an observable `NOT_SERVING` status.
- **The DSN wins for the target, the config wins for the rest.** With `connectionURI` set, the address, database, and credentials come from
  the DSN; pool sizing, timeouts, compression, settings, and an injected TLS config are applied on top.
- **An unknown compression value leaves the driver default in place** rather than failing — but `clickhouseconfig.Config.Validate` rejects it
  first, so this only matters when options are built from a config that was never validated.

## See also

- [`data/audit/storages/clickhouse`](../../../data/audit/storages/clickhouse) — audit storage that consumes this connection.
- [`infrastructure/mongo/factory`](../../mongo/factory) — same pattern for MongoDB.
