# nats

NATS infrastructure for the Atlas framework. Provides configuration-based creation of NATS connections and JetStream consumer
configurations with NKey, token, or username/password authentication, TLS, compression, and automatic reconnection.

## Subpackages

| Package              | Description                                                                            |
|----------------------|----------------------------------------------------------------------------------------|
| [factory](./factory) | Configuration-based NATS connection and JetStream `ConsumerConfig` creation            |
