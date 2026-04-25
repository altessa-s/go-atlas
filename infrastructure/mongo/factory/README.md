# factory

```go
import "github.com/altessa-s/go-atlas/infrastructure/mongo/factory"
```

Package `factory` provides a fluent builder for creating MongoDB clients from configuration.
`MongoBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
m, err := factory.New(cfg.Mongodb).
    UseLogger(logger).
    UseTlsConfig(tlsConfig).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
if err != nil {
    return err
}
if err := m.Connect(ctx); err != nil {
    return err
}
```

The returned `*mongo.Mongo` is not yet connected — call `Connect` to establish the connection.

## Authentication

| Mechanism     | Config field                | Description                                    |
|---------------|-----------------------------|------------------------------------------------|
| SCRAM-SHA-1   | `Credentials.Scram`         | Username, password, and auth source            |
| SCRAM-SHA-256 | `Credentials.Scram`         | Same fields, stronger hash                     |
| X.509         | `Credentials.AuthMechanism` | Certificate-based, no username/password needed |
| PLAIN         | `Credentials.Plain`         | LDAP proxy authentication                      |

## Methods

### Constructor

| Method     | Description                                                    |
|------------|----------------------------------------------------------------|
| `New(cfg)` | Creates a `MongoBuilder` for the given MongoDB config          |

### Dependencies

| Method                 | Description                                                     |
|------------------------|-----------------------------------------------------------------|
| `UseLogger`            | Sets the logger for the builder and all created components      |
| `UseTlsConfig`         | Sets the TLS configuration for secure connections               |
| `UseKmsProvider`       | Sets the KMS provider for client-side field level encryption    |
| `UseHealthCoordinator` | Registers a health checker under service name `"mongo"`         |

### Terminal

| Method       | Description                                                                            |
|--------------|----------------------------------------------------------------------------------------|
| `Build(ctx)` | Assembles and returns the `*mongo.Mongo` wrapper (not yet connected)                   |

## Configuration Modes

When `ConnectionURI` is set, `ApplyURI` is used as the base and pool/timeout/retry/TLS fields are layered on top. Otherwise, options are built from individual config fields including hosts, credentials, replica set, compressors, and direct connection flag.

When `Encryption` is configured, CSFLE auto-encryption is applied in bypass mode so reads transparently decrypt while writes use the explicit encryption path.
