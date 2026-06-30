# mtls (gRPC)

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"
```

gRPC adapter for mutual-TLS authentication. `AuthFunc` returns an `auth.AuthFunc` that extracts the verified peer certificate from the
connection and delegates identity derivation, validation, and audit to a [`auth/mtls.Authenticator`](../../../../../auth/mtls). Configure
behavior with that core package's options; this adapter adds the `grpc` transport label and maps failures to gRPC status codes.

## API

| Symbol                    | Description                                                                                       |
|---------------------------|--------------------------------------------------------------------------------------------------|
| `AuthFunc(opts…)`         | `auth.AuthFunc`; `opts` are `auth/mtls` options. Unauthenticated peer / rejected cert → `Unauthenticated`; required-audit failure → `Internal`. |
| `PeerCertificate(ctx)`    | Verified leaf client certificate from the context (verified chain, not unverified peer certs).    |

Errors from `PeerCertificate`: `ErrNoPeer`, `ErrNoTLS`, `ErrNoVerifiedCert` (`errors.Is`).

## Usage

```go
import (
    coremtls "github.com/altessa-s/go-atlas/auth/mtls"
    grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"
)

interceptor := grpcauth.ServerInterceptor(
    grpcauth.WithAuthFn(mtls.AuthFunc(                              // peer cert → spiffe.ID
        coremtls.WithValidator(coremtls.ExpiryValidator(nil, 30*time.Second)),
        coremtls.WithValidator(coremtls.TrustDomainValidator("example.org")),
        coremtls.WithAudit(rec, func(p any) string { id, _ := p.(spiffe.ID); return id.String() }),
    )),
    grpcauth.WithClientAuth(grpcauth.ScopeClientAuth(enf)),         // authorize on the principal
)
```

The TLS configuration that requires and verifies client certificates is set up separately — see
[`security/tlsutils`](../../../../../security/tlsutils).

## See also

- [`auth/mtls`](../../../../../auth/mtls) — the policy core (identity, validators, audit).
- [`auth/spiffe`](../../../../../auth/spiffe) — SPIFFE ID parsing.
- [`transport/http/server/middlewares/auth/mtls`](../../../../../transport/http/server/middlewares/auth/mtls) — the HTTP counterpart.
