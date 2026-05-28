# mongo

MongoDB infrastructure for the Atlas framework. Provides a fluent builder for creating MongoDB driver clients and higher-level
`mongo.Mongo` wrappers with authentication, TLS, connection pooling, compression, and client-side field level encryption (CSFLE).

## Usage

```go
m, err := factory.New(cfg.Mongodb).
    UseLogger(logger).
    UseTlsConfig(tlsConfig).
    UseKmsProvider(kmsProvider).
    UseHealthCoordinator(coordinator).
    Build(ctx)
if err != nil {
    return err
}
if err := m.Connect(ctx); err != nil {
    return err
}
```

## Features

- **Dual config paths** — connection-URI based (`ConnectionURI`) or field-based (hosts, credentials, pool settings), converging in
  `MongoBuilder.ClientOptions`
- **Authentication** — X.509, PLAIN (LDAP proxy), SCRAM-SHA-1, and SCRAM-SHA-256
- **TLS** — delegated via `UseTlsConfig`; applied to both URI and field-based paths
- **Compression** — configurable compressors with optional zlib compression level
- **CSFLE** — client-side field level encryption with pluggable KMS providers; auto-encryption in bypass mode for transparent read decryption
- **Health integration** — optional `health.Coordinator` registration via `UseHealthCoordinator`

## Subpackages

| Package              | Description                                                                       |
|----------------------|-----------------------------------------------------------------------------------|
| [factory](./factory) | Configuration-based `mongo.Mongo` and `mongo/options.ClientOptions` creation      |
