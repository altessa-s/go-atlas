# redisconfig

```go
import redisconfig "github.com/altessa-s/go-atlas/config/redis"
```

Package `redisconfig` defines the Redis connection schema. Schemas are populated by [`config/loader`](../loader) and consumed by the component
factories, which map them to generated options; runtime packages never import them.

## Key types

| Type    | Description                                                  |
|---------|--------------------------------------------------------------|
| `Config` | Represents the configuration for Redis database connections. |

See the [config index](../README.md) for the other schema packages.
