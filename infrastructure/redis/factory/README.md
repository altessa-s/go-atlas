# factory

```go
import "github.com/altessa-s/go-atlas/infrastructure/redis/factory"
```

Package `factory` provides a fluent builder for creating Redis clients from configuration.
`ClientBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
client, err := factory.New(cfg.Redis).
    UseLogger(logger).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
```

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
| `UseHealthCoordinator`   | Registers a health checker under service name `"redis"`         |

### Terminal

| Method        | Description                                                                                    |
|---------------|------------------------------------------------------------------------------------------------|
| `Build(ctx)`  | Assembles the `redis.UniversalClient`, pings it within `DefaultPingTimeout`, and returns it    |

## Constants

| Constant             | Value | Description                                           |
|----------------------|-------|-------------------------------------------------------|
| `DefaultPoolTimeout` | 10s   | Timeout for acquiring a connection from the pool      |
| `DefaultMaxRetries`  | 5     | Maximum retries for failed commands                   |
| `DefaultPingTimeout` | 5s    | Timeout for the initial health-check ping on creation |

## Configuration Modes

When `ConnectionURI` is set, it is parsed via `redis.ParseURL` to extract address, auth, TLS, and database number; pool, timeout, sentinel, and routing fields from config are applied on top. Otherwise, options are built from individual config fields.
