# clickhouseconfig

```go
import clickhouseconfig "github.com/altessa-s/go-atlas/config/clickhouse"
```

Package `clickhouseconfig` defines the ClickHouse connection schema. It is populated by [`config/loader`](../loader) and consumed by
[`infrastructure/clickhouse/factory`](../../infrastructure/clickhouse/factory), which opens the connection; runtime packages never import it.

## Key types

| Type          | Description                                                                          |
|---------------|--------------------------------------------------------------------------------------|
| `Config`      | Hosts or a `connectionURI` DSN, credentials, TLS, pool sizing, timeouts, compression |
| `Compression` | Wire compression: `none`, `lz4` (default), `zstd`, `gzip`, `deflate`, `br`           |

With `connectionURI` set, `hosts`, `database`, `username` and `password` come from the DSN and setting `username` or `password` as well is
a validation error. An empty `username` uses the server's `default` user. See the [template](../templates/clickhouse.yaml).

See the [config index](../README.md) for the other schema packages.
