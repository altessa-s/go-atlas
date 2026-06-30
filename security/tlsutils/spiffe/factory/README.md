# factory

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/spiffe/factory"
```

Builds a SPIFFE Workload API source from a [`config.SPIFFE`](../../../../config/spiffe.go). It derives the peer authorizer and core options,
mirroring the factory subpackages across the auth stack.

## Methods

| Method                    | Description                                                                            |
|---------------------------|----------------------------------------------------------------------------------------|
| `New(*config.SPIFFE)`     | Constructs the builder.                                                                 |
| `Authorizer()`            | Derives the `tlsconfig.Authorizer`: accept a peer in any trust domain or matching any ID. |
| `Options()`               | Returns the `spiffe.Option` slice (authorizer + socket path).                           |
| `Provider(ctx, extra...)` | Builds a connected `spiffe.Provider`; blocks until the first SVID arrives.              |

## Usage

```go
provider, err := factory.New(&cfg.SPIFFE).Provider(ctx)
if err != nil {
    return err
}
defer provider.Close()

server := &http.Server{TLSConfig: provider.MTLSServerConfig()}
```

The authorizer is fail-closed: a configuration with neither `allowedTrustDomains` nor `allowedIDs` returns `ErrNoAuthorizerSource`.
