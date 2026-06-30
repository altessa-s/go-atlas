# mtls (HTTP)

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
```

HTTP adapter for mutual-TLS authentication. `Middleware` reads the verified peer certificate from the request's TLS state, derives a
principal through a [`auth/mtls.Authenticator`](../../../../../../auth/mtls), and installs it into the request context with
`auth.ContextWithPrincipal` so downstream authorization (e.g. `auth.ScopeMiddleware`) reads it via `FromContext`. Configure behavior with
the core package's options; this adapter sets the `http` transport label.

## API

| Symbol                    | Description                                                                                    |
|---------------------------|------------------------------------------------------------------------------------------------|
| `Middleware(opts…)`       | `func(http.Handler) http.Handler`; `opts` are `auth/mtls` options. No/ rejected cert → `401`; required-audit failure → `500`. |
| `PeerCertificate(r)`      | Verified leaf client certificate from `r.TLS.VerifiedChains` (not unverified peer certs).       |

Errors from `PeerCertificate`: `ErrNoTLS`, `ErrNoVerifiedCert` (`errors.Is`).

## Usage

```go
import (
    coremtls "github.com/altessa-s/go-atlas/auth/mtls"
    httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
    "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
)

authn := mtls.Middleware(
    coremtls.WithValidator(coremtls.ExpiryValidator(nil, 30*time.Second)),
    coremtls.WithValidator(coremtls.TrustDomainValidator("example.org")),
)
authz := httpauth.ScopeMiddleware(enf, func(r *http.Request) string { return r.Method + " " + r.Pattern })

handler = authn(authz(handler)) // authn populates the principal authz reads
```

The TLS configuration that requires and verifies client certificates is set up separately — see
[`security/tlsutils`](../../../../../../security/tlsutils).

## See also

- [`auth/mtls`](../../../../../../auth/mtls) — the policy core (identity, validators, audit).
- [`transport/grpc/interceptors/auth/mtls`](../../../../../../transport/grpc/interceptors/auth/mtls) — the gRPC counterpart.
