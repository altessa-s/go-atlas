# nats

NATS infrastructure for the Atlas framework. Provides a fluent builder for creating NATS connections and JetStream consumer
configurations with NKey, token, or username/password authentication, TLS, compression, and automatic reconnection.

## Usage

```go
conn, err := factory.New(cfg.Nats).
    UseLogger(logger).
    UseTlsConfig(tlsConfig).
    UseHealthCoordinator(coordinator).
    Build()
```

## Features

- **Dual config paths** — connection-URI based (`ConnectionURI`) or field-based (hosts, credentials), converging in `ConnectionBuilder.NatsOptions`
- **Authentication** — NKey (Ed25519 seed), static token, or username/password; ignored when using a connection URI
- **TLS** — optional secure transport via `UseTlsConfig`
- **Reconnection** — unlimited reconnects and unlimited reconnect buffer by default; configurable via `MaxReconnect` and `ReconnectWait`
- **JetStream consumers** — `ConsumerConfig` maps `config.NatsConsumer` to `jetstream.ConsumerConfig` with delivery, ack, and replay policies
- **Health integration** — optional `health.Coordinator` registration via `UseHealthCoordinator`

## Subpackages

| Package              | Description                                                                            |
|----------------------|----------------------------------------------------------------------------------------|
| [factory](./factory) | Configuration-based NATS connection and JetStream `ConsumerConfig` creation            |
