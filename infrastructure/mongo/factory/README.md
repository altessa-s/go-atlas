# factory

```go
import "github.com/altessa-s/go-atlas/infrastructure/mongo/factory"
```

Package `factory` provides configuration-based creation of MongoDB clients. Reads from `config.Mongodb`
to produce driver-level `ClientOptions` and higher-level `mongo.Mongo` wrappers with authentication,
TLS, connection pooling, and CSFLE encryption.

## Factory methods

| Method                            | Description                                                            |
|-----------------------------------|------------------------------------------------------------------------|
| `ClientOptionsFromConfig`         | Build driver `ClientOptions` from config (URI-based or field-based)    |
| `CreateMongoOptionsFromConfig`    | Build `[]mongo.Option` including KMS and CSFLE settings for `mongo.New`|
| `CreateMongoFromConfig`           | Create a `mongo.Mongo` wrapper ready for `Connect`; register health   |
| `CreateMongoFromConfigWithContext` | Same as above with explicit context                                   |

## Options

| Option                   | Description                                                         |
|--------------------------|---------------------------------------------------------------------|
| `WithLogger`             | Set the `*slog.Logger` for the factory (default: discard)           |
| `WithHealthCoordinator`  | Register a health checker under service name `"mongo"`              |
| `WithTlsFactory`         | Delegate TLS setup to `security/tlsutils/factory`                   |
| `WithKmsFactory`         | Delegate KMS/CSFLE setup to `data/mongo/kms/factory`               |

## Authentication

| Mechanism    | Config field                | Description                                      |
|--------------|-----------------------------|--------------------------------------------------|
| SCRAM-SHA-1  | `Credentials.Scram`         | Username, password, and auth source              |
| SCRAM-SHA-256| `Credentials.Scram`         | Same fields, stronger hash                       |
| X.509        | `Credentials.AuthMechanism` | Certificate-based, no username/password needed   |
| PLAIN        | `Credentials.Plain`         | LDAP proxy authentication                        |

## Configuration modes

When `ConnectionURI` is set, `ApplyURI` is used as the base and pool/timeout/retry/TLS fields are
layered on top. Otherwise, options are built from individual config fields including hosts, credentials,
replica set, compressors, and direct connection flag.
