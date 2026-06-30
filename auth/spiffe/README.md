# spiffe

```go
import "github.com/altessa-s/go-atlas/auth/spiffe"
```

Parses [SPIFFE IDs](https://spiffe.io/) from X.509 certificates. A pure, transport-free primitive: given a verified leaf certificate (or a
raw URI), it returns the workload's `ID` — the trust domain and path of an `spiffe://trust-domain/path` identity. It carries no transport or
TLS concern; the [mTLS gRPC adapter](../../transport/grpc/interceptors/auth/mtls) wires it into the authentication seam.

## Key types

| Type | Description                                                                                   |
|------|-----------------------------------------------------------------------------------------------|
| `ID` | Parsed SPIFFE ID: `TrustDomain` (lowercase authority) and `Path` (workload path with leading `/`). |

## Functions

| Function                      | Description                                                                                   |
|-------------------------------|-----------------------------------------------------------------------------------------------|
| `ParseID(raw)`                | Parse `spiffe://trust-domain/path`; enforces scheme, non-empty trust domain, no port/query/fragment. |
| `IDFromCertificate(cert)`     | Extract the SPIFFE ID from a leaf cert's single URI SAN (the X.509-SVID rule).                  |
| `ID.String()`                 | Canonical `spiffe://…` form, or `""` for the zero ID.                                          |
| `ID.IsZero()`                 | Whether the ID is empty.                                                                       |

Errors: `ErrInvalidID`, `ErrNoSVID` (no URI SAN), `ErrMultipleURIs` (more than one URI SAN — never valid for an SVID). Match with
`errors.Is`.

## Usage

```go
id, err := spiffe.IDFromCertificate(cert) // cert is a verified *x509.Certificate
if err != nil {
    return err
}
_ = id.TrustDomain // "example.org"
_ = id.Path        // "/ns/default/sa/billing"
_ = id.String()    // "spiffe://example.org/ns/default/sa/billing"
```

## See also

- [`transport/grpc/interceptors/auth/mtls`](../../transport/grpc/interceptors/auth/mtls) — gRPC interceptor that derives a principal from
  the verified mTLS client certificate.
- [`docs/auth/README.md`](../../docs/auth/README.md) — auth subsystem overview.
