# factory

```go
import "github.com/altessa-s/go-atlas/factory/infrastructure/redis"
```

Package `factory` provides a fluent builder for creating Redis clients from configuration. `ClientBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
owner, err := factory.New(cfg.Redis).
    UseLogger(logger).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
```

After a successful build, call `owner.Close()` or register `UseShutdownHooks(group)`. Close synchronously removes its own health checker and closes the
driver once. A later registration under the same name survives.

`owner.Client()` returns a borrowed reference to the raw driver client. Storages and storage factories must never close it. Type assertions on the
borrowed client expose driver-specific operations. For a Sentinel-backed cluster client, pass `owner.SentinelCluster()` to the Bloom or Cuckoo storage
factory’s `SentinelBuilder`.

## Supported Modes

Mode is detected automatically from configuration — no explicit mode field is required.

| Mode       | Condition                                             |
|------------|-------------------------------------------------------|
| Standalone | Single host and no `MasterName`                       |
| Sentinel   | `MasterName` is set                                   |
| Cluster    | Multiple hosts provided and `MasterName` is empty     |

## Methods

### Constructor

| Method     | Description                                           |
|------------|-------------------------------------------------------|
| `New(cfg)` | Creates a `ClientBuilder` for the given Redis config  |

### Dependencies

| Method                   | Description                                                     |
|--------------------------|-----------------------------------------------------------------|
| `UseLogger`              | Sets the logger for the builder and all created components      |
| `UseShutdownHooks`       | Registers the owner’s `Close` with the supplied shutdown hook group |
| `UseDefaultLogger`       | Shorthand for `UseLogger(slog.Default())`                       |
| `UseHealthCoordinator`   | Registers a health checker for the created client               |
| `UseHealthServiceName`   | Name to register the checker under (default `"redis"`)          |
| `WithTLSConfig`          | Applies custom `*tls.Config` on field-based and `rediss://` URI configs; rejected on non-TLS URIs |

### Startup behavior

| Method            | Description                                                                                  |
|-------------------|----------------------------------------------------------------------------------------------|
| `SkipStartupPing` | Skips the startup ping, so `Build` succeeds while Redis is down; go-redis reconnects by itself |
| `WithoutHealth`   | Does not register a health checker, even when a coordinator is set                           |

Together they make Redis optional for the service, for example behind a fail-open cache:

```go
owner, err := factory.New(cfg.Redis).
    SkipStartupPing().
    WithoutHealth().
    Build(ctx)
```

### Terminal

| Method        | Description                                                                                    |
|---------------|------------------------------------------------------------------------------------------------|
| `Build(ctx)`  | Assembles the `redis.UniversalClient`, pings it within `DefaultPingTimeout` (unless skipped), and returns its `*ManagedClient` owner |
| `UniversalOptions()` | Renders the `redis.UniversalOptions` without creating a client                                 |

The startup ping uses the earlier of the caller's deadline and
`DefaultPingTimeout`. On failure or cancellation, the builder closes the
client and returns the ping failure together with any close error.

## go-redis logging

To route driver diagnostics, including Sentinel discovery, through your
service logger, call this once during service startup:

```go
redis.SetLogger(factory.NewSlogAdapter(logger))
```

Here `redis` is `github.com/redis/go-redis/v9`. The adapter emits a generic
warning to keep driver diagnostic text and server addresses private. Use a
nonblocking handler because go-redis can log while holding locks.

## Constants

| Constant             | Value | Description                                           |
|----------------------|-------|-------------------------------------------------------|
| `DefaultPoolTimeout` | 10s   | Timeout for acquiring a connection from the pool      |
| `DefaultMaxRetries`  | 5     | Maximum retries for failed commands                   |
| `DefaultPingTimeout` | 5s    | Timeout for the initial health-check ping on creation |

## Configuration Modes

When `ConnectionURI` is set, it is parsed via `redis.ParseURL` to extract address, auth, TLS, and database number; pool, timeout, sentinel, and routing
fields from config are applied on top. Otherwise, options are built from individual config fields.
URI parsing failures match `redisconfig.ErrInvalidConnectionURI` and omit the
URI and parser diagnostics.
