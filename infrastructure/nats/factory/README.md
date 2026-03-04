# factory

```go
import "github.com/altessa-s/go-atlas/infrastructure/nats/factory"
```

Package `factory` provides a fluent builder for creating NATS connections from configuration.
`ConnectionBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
conn, err := factory.New(cfg.Nats).
    UseLogger(logger).
    UseTlsConfig(tlsConfig).
    UseHealthCoordinator(healthCoordinator).
    Build()
```

## Authentication

Authentication is selected automatically based on which credential fields are set in `config.Nats`. When `ConnectionURI` is set, authentication fields are ignored because credentials are embedded in the URI.

| Method            | Config field            | Description                                            |
|-------------------|-------------------------|--------------------------------------------------------|
| NKey              | `NkeySeed`              | Ed25519-based authentication via seed and derived key  |
| Token             | `Token`                 | Static token authentication                            |
| Username/Password | `Username` + `Password` | Basic credential authentication                        |

## JetStream Consumer Policies

| Policy type    | Supported values                                        |
|----------------|---------------------------------------------------------|
| Deliver policy | `All`, `Last`, `New`, `ByStartSequence`, `ByStartTime` |
| Ack policy     | `None`, `All`, `Explicit`                               |
| Replay policy  | `Instant`, `Original`                                   |

## Methods

### Constructor

| Method     | Description                                                  |
|------------|--------------------------------------------------------------|
| `New(cfg)` | Creates a `ConnectionBuilder` for the given NATS config      |

### Dependencies

| Method                 | Description                                                     |
|------------------------|-----------------------------------------------------------------|
| `UseLogger`            | Sets the logger for the builder and all created components      |
| `UseTlsConfig`         | Sets the TLS configuration for secure connections               |
| `UseHealthCoordinator` | Registers a health checker under service name `"nats"`          |

### Terminal

| Method    | Description                                                                                    |
|-----------|------------------------------------------------------------------------------------------------|
| `Build()` | Assembles and returns the `*nats.Conn`                                                         |

## Constants

| Constant                   | Value | Description                                          |
|----------------------------|-------|------------------------------------------------------|
| `UnlimitedReconnects`      | -1    | Reconnect indefinitely after connection loss         |
| `UnlimitedReconnectBuffer` | -1    | Buffer messages without limit during reconnection    |
