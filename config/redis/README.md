# redisconfig

```go
import redisconfig "github.com/altessa-s/go-atlas/config/redis"
```

Package `redisconfig` defines the Redis connection schema. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories, which map them to generated
options; runtime packages never import them.

## Key types

| Type    | Description                                                  |
|---------|--------------------------------------------------------------|
| `Config` | Represents the configuration for Redis database connections. |

`Config.Validate` checks `connectionURI` with the go-redis URL parser. A
parse failure matches `ErrInvalidConnectionURI` via `errors.Is`. Its message
omits the URI and parser diagnostics.

See the [config index](../README.md) for the other schema packages.
