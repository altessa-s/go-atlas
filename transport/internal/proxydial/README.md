# proxydial

```go
import "github.com/altessa-s/go-atlas/transport/internal/proxydial"
```

Package `proxydial` holds the wire-protocol details for routing client connections through a forward proxy: HTTP CONNECT (with optional TLS to the proxy itself), SOCKS5, and the `*tls.Config` merge rules used for the TLS handshake to an `https://` proxy.

Internal package — consumed by `transport/http/client` and `transport/grpc/client` to keep their proxy dialers in sync.

## Key functions

| Symbol                 | Description                                                                                    |
|------------------------|------------------------------------------------------------------------------------------------|
| `HTTPConnect`          | Open TCP+TLS to the proxy and send a CONNECT for `addr`; return the tunneled `net.Conn`        |
| `SOCKS5Dialer`         | Construct a SOCKS5 dialer that opens TCP to `addr` through the proxy                           |
| `TLSConfig`            | Clone user's `*tls.Config` and fill `ServerName`/`MinVersion` from URL when zero               |
| `DefaultDialer`        | `*net.Dialer` pre-configured with `DefaultDialTimeout` and `DefaultDialKeepAlive`              |
| `DefaultDialTimeout`   | Default TCP connect timeout (30s) shared by HTTP and gRPC client transports                    |
| `DefaultDialKeepAlive` | Default TCP keep-alive interval (30s) shared by HTTP and gRPC client transports                |
| `DialContextFunc`      | Type alias for `func(ctx, network, addr) (net.Conn, error)` (matches `net.Dialer.DialContext`) |

## Usage

Wrapped per-host-transport because each consumer's dialer signature differs (gRPC: `(ctx, addr)`, HTTP: `(ctx, network, addr)`):

```go
// transport/grpc/client
return func(ctx context.Context, addr string) (net.Conn, error) {
    return proxydial.HTTPConnect(ctx, proxydial.DefaultDialer(), proxyURL, tlsCfg, addr)
}

// transport/http/client
cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
    proxyURL, _ := proxyFn(req)
    return proxydial.HTTPConnect(ctx, proxydial.DefaultDialer(), proxyURL, tlsCfg, addr)
}
```
