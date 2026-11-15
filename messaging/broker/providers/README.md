# providers

Broker provider implementations. Each subpackage implements the `broker.Provider` interface for a specific messaging backend.

## Subpackages

| Package        | Description                                                                       |
|----------------|-----------------------------------------------------------------------------------|
| [nats](./nats) | NATS JetStream implementation of `broker.Provider` with stream and consumer mgmt  |
