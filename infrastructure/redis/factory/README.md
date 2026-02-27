# factory

```go
import "github.com/altessa-s/go-atlas/infrastructure/redis/factory"
```

Package `factory` provides configuration-based creation of Redis clients. Reads from `config.Redis`
to create `redis.UniversalClient` instances with authentication, connection pooling, and automatic
mode detection (standalone, sentinel, cluster).

## Factory methods

| Method                      | Description                                                                     |
|-----------------------------|---------------------------------------------------------------------------------|
| `UniversalOptionsFromConfig`| Build `redis.UniversalOptions` from config (URI-based or field-based)           |
| `CreateClientFromConfig`    | Create a `redis.UniversalClient`, ping it, and optionally register health check |

## Options

| Option                   | Description                                                         |
|--------------------------|---------------------------------------------------------------------|
| `WithLogger`             | Set the `*slog.Logger` for the factory (default: discard)           |
| `WithHealthCoordinator`  | Register a health checker under service name `"redis"`              |

## Mode detection

The client mode is determined automatically from the configuration -- no explicit mode field is
required.

| Mode       | Condition                                                  |
|------------|------------------------------------------------------------|
| Sentinel   | `MasterName` is set                                        |
| Cluster    | Multiple hosts are provided and `MasterName` is empty      |
| Standalone | Single host and no `MasterName`                            |

## Constants

| Constant             | Value    | Description                                             |
|----------------------|----------|---------------------------------------------------------|
| `DefaultPoolTimeout` | 10s      | Timeout for acquiring a connection from the pool        |
| `DefaultMaxRetries`  | 5        | Maximum retries for failed commands                     |
| `DefaultPingTimeout` | 5s       | Timeout for the initial health-check ping on creation   |

## Configuration modes

When `ConnectionURI` is set, it is parsed via `redis.ParseURL` to extract address, auth, TLS, and
database number; pool, timeout, sentinel, and routing fields from config are applied on top. Otherwise,
options are built from individual config fields.
