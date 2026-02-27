# factory

```go
import "github.com/altessa-s/go-atlas/infrastructure/nats/factory"
```

Package `factory` provides configuration-based creation of NATS connections and JetStream consumer
configurations. Reads from `config.Nats` and `config.NatsConsumer` to create connections with
authentication, TLS, compression, and unlimited reconnection.

## Factory methods

| Method                       | Description                                                                     |
|------------------------------|---------------------------------------------------------------------------------|
| `NatsOptionsFromConfig`      | Build `[]nats.Option` from config (timeouts, auth, TLS, reconnect, compression) |
| `CreateConnectionFromConfig` | Create a `*nats.Conn` and optionally register a health check under `"nats"`     |
| `ConsumerConfigFromConfig`   | Convert `config.NatsConsumer` to `jetstream.ConsumerConfig`                     |

## Options

| Option                   | Description                                                         |
|--------------------------|---------------------------------------------------------------------|
| `WithLogger`             | Set the `*slog.Logger` for the factory (default: discard)           |
| `WithHealthCoordinator`  | Register a health checker under service name `"nats"`               |
| `WithTlsFactory`         | Delegate TLS setup to `security/tlsutils/factory`                   |

## Authentication

Authentication is selected automatically based on which credential fields are set in `config.Nats`.
When `ConnectionURI` is set, authentication fields are ignored because credentials are embedded in
the URI.

| Method            | Config field            | Description                                          |
|-------------------|-------------------------|------------------------------------------------------|
| NKey              | `NkeySeed`              | Ed25519-based authentication via seed and derived key |
| Token             | `Token`                 | Static token authentication                          |
| Username/Password | `Username` + `Password` | Basic credential authentication                      |

## JetStream consumer policies

| Policy type    | Supported values                                                        |
|----------------|-------------------------------------------------------------------------|
| Deliver policy | `All`, `Last`, `New`, `ByStartSequence`, `ByStartTime`                 |
| Ack policy     | `None`, `All`, `Explicit`                                              |
| Replay policy  | `Instant`, `Original`                                                  |

## Constants

| Constant                  | Value | Description                                         |
|---------------------------|-------|-----------------------------------------------------|
| `UnlimitedReconnects`     | -1    | Reconnect indefinitely after connection loss        |
| `UnlimitedReconnectBuffer`| -1    | Buffer messages without limit during reconnection   |
