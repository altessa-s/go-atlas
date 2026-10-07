# tlsconfig

```go
import tlsconfig "github.com/altessa-s/go-atlas/config/tls"
```

Package `tlsconfig` defines TLS client and server schemas and the certificate provider schemas (file, Let's Encrypt, OCSP, S3, Vault). Schemas are
populated by [`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import
them.

## Key types

| Type                     | Description                                                                                                            |
|--------------------------|------------------------------------------------------------------------------------------------------------------------|
| `SkipVerifyMode`      | Controls how the TLS factory reacts when a [Client] config has SkipVerify=true.                                     |
| `Client`              | Represents the configuration for Tls client connections.                                                               |
| `ProviderType`        | Defines the type of Tls certificate provider.                                                                          |
| `Provider`            | Represents the configuration for Tls certificate providers.                                                            |
| `ProviderFile`        | Configures file-based Tls certificate provider.                                                                        |
| `ProviderLetsEncrypt` | Configures Let's Encrypt ACME certificate provider.                                                                    |
| `ProviderOCSP`        | Configures the OCSP stapler used by the TLS providers registered through [security/tlsutils/factory.ProvidersBuilder]. |
| `SSETypeConfig`          | Identifies the server-side encryption type for S3 objects in configuration.                                            |
| `ProviderS3`          | Configures S3-based TLS certificate provider.                                                                          |
| `ProviderS3SSE`       | Configures server-side encryption for S3 certificate objects.                                                          |
| `ProviderVault`       | Configures HashiCorp Vault PKI certificate provider.                                                                   |
| `ClientAuth`          | Represents the configuration for client certificate authentication.                                                    |
| `Server`              | Represents the configuration for Tls server settings.                                                                  |

See the [config index](../README.md) for the other schema packages.
