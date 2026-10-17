# providers

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/providers"
```

Package `tlsproviders` defines interfaces for TLS certificate providers and a registry for managing multiple providers.

## Key types

| Type / Interface    | Description                                       |
|---------------------|---------------------------------------------------|
| `Provider`          | Interface: `Type()`, `TLSConfig()`, `Close(ctx)`  |
| `Certificate`       | Dynamic certificate selection via `GetCertificate` |
| `ClientCertificate` | Client cert provider via `GetClientCertificate`   |
| `Providers`         | Registry: `Register`, `Get`, `List`, `Close`      |
| `ProviderType`      | Provider identifier (`file`, `vault`, `letsencrypt`, `s3`) |

## Subpackages

| Package                    | Description                              |
|----------------------------|------------------------------------------|
| [file](./file)             | File-based certs with optional reload    |
| [letsencrypt](./letsencrypt) | Let's Encrypt ACME integration           |
| [s3](./s3)                 | S3-backed certs with optional reload     |
| [vault](./vault)           | Vault PKI engine for dynamic certs       |
