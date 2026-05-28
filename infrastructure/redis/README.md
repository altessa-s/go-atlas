# redis

Redis infrastructure for the Atlas framework. Provides a fluent builder for creating Redis clients with automatic mode detection
(standalone, sentinel, cluster), authentication, connection pooling, and health-check integration.

## Usage

```go
client, err := factory.New(cfg.Redis).
    UseLogger(logger).
    UseHealthCoordinator(coordinator).
    Build(ctx)
```

## Features

- **Dual config paths** — connection-URI based (`ConnectionURI`, parsed via `redis.ParseURL`) or field-based (hosts, credentials, pool settings),
  converging in `ClientBuilder.UniversalOptions`
- **Auto mode detection** — sentinel when `MasterName` is set, cluster when multiple hosts are provided, standalone otherwise
- **Authentication** — username/password with optional sentinel password
- **Connection pooling** — configurable pool size, idle connections, timeouts, and max connection age
- **Startup health check** — `Build` pings the server within `DefaultPingTimeout` and closes the client on failure
- **Health integration** — optional `health.Coordinator` registration via `UseHealthCoordinator`

## Subpackages

| Package              | Description                                                                       |
|----------------------|-----------------------------------------------------------------------------------|
| [factory](./factory) | Configuration-based `redis.UniversalClient` creation with mode auto-detection     |
