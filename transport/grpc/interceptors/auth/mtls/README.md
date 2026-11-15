# mtls (gRPC)

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"
```

gRPC adapter for mutual-TLS authentication. `AuthFunc` returns an `auth.AuthFunc` that extracts the verified peer certificate from the connection and
delegates identity derivation, validation, and audit to a [`auth/mtls.Authenticator`](../../../../../auth/mtls). Configure behavior with that core
package's options; this adapter adds the `grpc` transport label and maps failures to gRPC status codes.

## API

| Symbol                    | Description                                                                                       |
|---------------------------|--------------------------------------------------------------------------------------------------|
| `AuthFunc(opts…)`         | `(auth.AuthFunc, error)`; `opts` are `auth/mtls` options. Errors with `coremtls.ErrNoTrustDomainPolicy` without a trust decision. Unauthenticated peer / rejected cert → `Unauthenticated`; required-audit failure → `Internal`. |
| `PeerCertificate(ctx)`    | Verified leaf client certificate from the context (verified chain, not unverified peer certs).    |

Errors from `PeerCertificate`: `ErrNoPeer`, `ErrNoTLS`, `ErrNoVerifiedCert` (`errors.Is`).

## Usage

```go
import (
    coremtls "github.com/altessa-s/go-atlas/auth/mtls"
    grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"
)

authFn, err := mtls.AuthFunc(                                       // peer cert → spiffe.ID
    coremtls.WithTrustDomains("example.org"),                       // the trust decision
    coremtls.WithValidator(coremtls.ExpiryValidator(nil, 30*time.Second)),
    coremtls.WithAudit(rec, func(p any) string { id, _ := p.(spiffe.ID); return id.String() }),
)
if err != nil {
    return err // coremtls.ErrNoTrustDomainPolicy: no trust decision
}
interceptor := grpcauth.ServerInterceptor(
    grpcauth.WithAuthFn(authFn),
    grpcauth.WithClientAuth(grpcauth.ScopeClientAuth(enf)),         // authorize on the principal
)
```

The options must make the trust decision — `coremtls.WithTrustDomains(domains…)` or the `coremtls.WithInsecureAnyTrustDomain()` opt-in. Validators
(`WithValidator`) never count as one: without the decision `AuthFunc` returns an error wrapping `coremtls.ErrNoTrustDomainPolicy` (match with
`errors.Is`), so the service fails at startup instead of answering `Unauthenticated` at runtime.

The TLS configuration that requires and verifies client certificates is set up separately — see
[`go-tlsutils`](https://github.com/altessa-s/go-tlsutils/tree/develop).

## See also

- [`auth/mtls`](../../../../../auth/mtls) — the policy core (identity, validators, audit).
- [`auth/spiffe`](../../../../../auth/spiffe) — SPIFFE ID parsing.
- [`transport/http/server/middlewares/auth/mtls`](../../../../../transport/http/server/middlewares/auth/mtls) — the HTTP counterpart.
