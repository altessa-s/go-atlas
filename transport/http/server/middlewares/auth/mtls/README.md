# mtls (HTTP)

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
```

HTTP adapter for mutual-TLS authentication. `Middleware` reads the verified peer certificate from the request's TLS state, derives a principal through a
[`auth/mtls.Authenticator`](../../../../../../auth/mtls), and installs it into the request context with `auth.ContextWithPrincipal` so downstream
authorization (e.g. `auth.ScopeMiddleware`) reads it via `FromContext`. Configure behavior with the core package's options; this adapter sets the `http`
transport label.

## API

| Symbol                    | Description                                                                                    |
|---------------------------|------------------------------------------------------------------------------------------------|
| `New(opts…)` / `Name()` | Constructs the ordering-aware middleware and identifies it as `mtls`. |
| `Middleware(opts…)`       | `(func(http.Handler) http.Handler, error)`; `opts` are `auth/mtls` options. Errors with `coremtls.ErrNoTrustDomainPolicy` without a trust decision. No/ rejected cert → `401`; required-audit failure → `500`. |
| `PeerCertificate(r)`      | Verified leaf client certificate from `r.TLS.VerifiedChains` (not unverified peer certs).       |

Errors from `PeerCertificate`: `ErrNoTLS`, `ErrNoVerifiedCert` (`errors.Is`).

## Usage

```go
import (
    coremtls "github.com/altessa-s/go-atlas/auth/mtls"
    httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
    "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
)

authn, err := mtls.Middleware(
    coremtls.WithTrustDomains("example.org"), // the trust decision
    coremtls.WithValidator(coremtls.ExpiryValidator(nil, 30*time.Second)),
)
if err != nil {
    return err // coremtls.ErrNoTrustDomainPolicy: no trust decision
}
authz := httpauth.ScopeMiddleware(enf, func(r *http.Request) string { return r.Method + " " + r.Pattern })

handler = authn(authz(handler)) // authn populates the principal authz reads
```

The options must make the trust decision — `coremtls.WithTrustDomains(domains…)` or the `coremtls.WithInsecureAnyTrustDomain()` opt-in. Validators
(`WithValidator`) never count as one: without the decision `Middleware` returns an error wrapping `coremtls.ErrNoTrustDomainPolicy` (match with
`errors.Is`), so the service fails at startup instead of answering `401` at runtime.

The TLS configuration that requires and verifies client certificates is set up separately — see
[`go-tlsutils`](https://github.com/altessa-s/go-tlsutils/tree/develop).

## See also

- [`auth/mtls`](../../../../../../auth/mtls) — the policy core (identity, validators, audit).
- [`transport/grpc/interceptors/auth/mtls`](../../../../../../transport/grpc/interceptors/auth/mtls) — the gRPC counterpart.
