# spiffe

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/spiffe"
```

Sources rotating mutual-TLS configurations from the SPIFFE Workload API. It is the producer half of mTLS — it keeps the service's own
X.509-SVID and trust bundle fresh — and the counterpart to the verifier-side [auth/spiffe](../../../auth/spiffe) and
[auth/mtls](../../../auth/mtls), which turn a peer's verified certificate into a principal.

## Options

| Option                  | Description                                                                                      |
|-------------------------|--------------------------------------------------------------------------------------------------|
| `WithAuthorizer(a)`     | Mandatory peer authorizer (`tlsconfig.Authorizer`). Without it `New` fails with `ErrNoAuthorizer`. |
| `WithSocketPath(addr)`  | Workload API endpoint (e.g. `unix:///run/spire/agent/api.sock`). Empty uses `SPIFFE_ENDPOINT_SOCKET`. |

## Key types

| Type        | Description                                                                                       |
|-------------|--------------------------------------------------------------------------------------------------|
| `Source`    | Seam: an `x509svid.Source` + `x509bundle.Source` + `io.Closer`. `workloadapi.X509Source` satisfies it. |
| `Provider`  | Builds `MTLSServerConfig()` / `MTLSClientConfig()` that pull the current SVID on every handshake. |

## Usage

```go
provider, err := spiffe.New(ctx,
    spiffe.WithSocketPath("unix:///run/spire/agent/api.sock"),
    spiffe.WithAuthorizer(tlsconfig.AuthorizeMemberOf(td)),
)
if err != nil {
    return err
}
defer provider.Close()

server := &http.Server{TLSConfig: provider.MTLSServerConfig()}     // inbound mTLS

// Outbound mTLS — provider.DialContext / HTTPTransport carry the rotating SVID.
client := &http.Client{Transport: provider.HTTPTransport()}
```

`Provider` exposes both sides: `MTLSServerConfig()` for inbound, and `MTLSClientConfig()` / `DialContext` / `HTTPTransport()` for outbound
calls. Server identity is verified by SPIFFE ID, not DNS name.

For gRPC, `ServerCredentials()` / `ClientCredentials()` return `credentials.TransportCredentials`:

```go
grpcServer := grpc.NewServer(grpc.Creds(provider.ServerCredentials()))
conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(provider.ClientCredentials()))
```

Derive the authorizer from configuration with the [factory](./factory) subpackage:

```go
provider, err := factory.New(&cfg.SPIFFE).Provider(ctx)
```

## Scope

Obtaining and rotating the SVID is all this package does. It deliberately does not parse principals out of certificates
([auth/spiffe](../../../auth/spiffe)) or make authorization decisions ([auth/mtls](../../../auth/mtls)). A peer authorizer is mandatory —
the provider is fail-closed and will not trust every SPIFFE ID.
