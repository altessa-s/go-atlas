# tlsutils

```go
import "github.com/altessa-s/go-atlas/security/tlsutils"
```

Package `tlsutils` provides TLS certificate loading and secure configuration utilities. Supports multiple certificate sources (file, Vault, Let's
Encrypt), key formats (PKCS#8, PKCS#1, SEC1), and TLS 1.2+ defaults.

## Functions

| Function                      | Description                              |
|-------------------------------|------------------------------------------|
| `LoadFromFile`                | Load cert + key from separate files      |
| `LoadFromConcatenatedFile`    | Load cert + key from a single PEM file   |
| `LoadFromBytes`               | Load cert + key from PEM bytes           |
| `ApplyMinTLSVersion`          | Apply a YAML `minVersion` (`1.2`/`1.3`) to a server `tls.Config`; TLS 1.3 clears the 1.2 cipher list |
| `ResolveMinTLSVersion`        | Parse a `minVersion` string; `ok=false` for unknown input |
| `DefaultTLSConfig`            | Secure TLS 1.2+ server configuration     |
| `DefaultTLSConfigStrict`      | TLS 1.3-only server configuration        |
| `DefaultClientTLSConfig`      | Secure TLS 1.2+ client configuration     |
| `DefaultClientTLSConfigStrict` | TLS 1.3-only client configuration       |
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
| [providers/letsencrypt](./providers/letsencrypt) | Let's Encrypt ACME integration         |
| [providers/s3](./providers/s3)         | S3-backed certificate loading          |
| [providers/vault](./providers/vault)   | Vault PKI engine certificates          |
| [spiffe](./spiffe)                     | Rotating mTLS configs from the SPIFFE Workload API |
