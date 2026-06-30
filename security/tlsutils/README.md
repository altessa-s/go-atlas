# tlsutils

```go
import "github.com/altessa-s/go-atlas/security/tlsutils"
```

Package `tlsutils` provides TLS certificate loading and secure configuration utilities. Supports multiple certificate sources (file, Vault,
Let's Encrypt), key formats (PKCS#8, PKCS#1, SEC1), and TLS 1.2+ defaults.

## Functions

| Function                      | Description                              |
|-------------------------------|------------------------------------------|
| `LoadFromFile`                | Load cert + key from separate files      |
| `LoadFromConcatenatedFile`    | Load cert + key from a single PEM file   |
| `DefaultTLSConfig`            | Secure TLS 1.2+ server configuration     |
| `DefaultClientTLSConfig`      | Secure TLS 1.2+ client configuration     |
| `ClientTLSConfig`             | Strict client mTLS config presenting a cert from a provider |
| `DialContext`                 | Dial a peer over mutual TLS using a client-cert provider |
| `BuildCAPool`                 | Build certificate pool from CA files     |
| `CloneCertificateWithOCSPStaple` | Attach OCSP staple to certificate     |

## Subpackages

| Package                                | Description                            |
|----------------------------------------|----------------------------------------|
| [factory](./factory)                   | Config-based TLS setup                 |
| [ocsp](./ocsp)                         | OCSP stapling with caching and refresh |
| [providers](./providers)               | Certificate provider registry          |
| [providers/file](./providers/file)     | File-based certificate loading         |
| [providers/le](./providers/le)         | Let's Encrypt ACME integration         |
| [providers/s3](./providers/s3)         | S3-backed certificate loading          |
| [providers/vault](./providers/vault)   | Vault PKI engine certificates          |
