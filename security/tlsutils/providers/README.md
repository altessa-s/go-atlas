# providers

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/providers"
```

Package `providers` defines interfaces for TLS certificate providers and a registry for managing
multiple providers.

## Key types

| Type / Interface    | Description                                       |
|---------------------|---------------------------------------------------|
| `Provider`          | Interface: `Type()`, `TLSConfig()`, `Close()`     |
| `Certificate`       | Dynamic certificate selection via `GetCertificate` |
| `ClientCertificate` | Client cert provider via `GetClientCertificate`   |
| `Providers`         | Registry for multiple providers                   |

## Subpackages

| Package                    | Description                              |
|----------------------------|------------------------------------------|
| [file](./file)             | File-based certs with optional reload    |
| [le](./le)                 | Let's Encrypt ACME integration           |
| [vault](./vault)           | Vault PKI engine for dynamic certs       |
