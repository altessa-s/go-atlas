# factory

```go
import "github.com/altessa-s/go-atlas/transport/proxydial/factory"
```

Package `factory` provides a fluent builder that materializes a [`config.Proxy`](../../../config/proxy.go)
into a [`proxydial.DialContextFunc`](../proxydial.go), suitable for any
client that needs raw-TCP-through-proxy: SMTP (go-mail's
`WithDialContextFunc`), SOAP, gRPC's `WithContextDialer`, or any
custom protocol that does not go through `net/http`.

HTTP and gRPC clients should keep using `cfg.Proxy.HTTPClientOptions()`
/ `cfg.Proxy.GrpcClientOptions()`; this factory targets consumers
without an httpclient/grpcclient in the picture.

## Quick start

```go
dial, err := factory.New(cfg.SMTP.Proxy).
    UseLogger(logger).
    UseProxyTLSConfig(tlsCfg).
    Build()
if err != nil {
    return nil, fmt.Errorf("build smtp proxy dialer: %w", err)
}
if dial != nil {
    mailOpts = append(mailOpts, mail.WithDialContextFunc(dial))
}
```

A nil `cfg`, an empty `Mode`, or `config.ProxyModeNone` yields
`(nil, nil)` — caller treats that as "use a direct dial" and skips
wiring a custom dialer entirely.

## Methods

### Constructor

| Method     | Description                                                            |
|------------|------------------------------------------------------------------------|
| `New(cfg)` | Creates a `DialerBuilder` for the given `*config.Proxy` (nil ok)       |

### Dependencies

| Method                 | Description                                                                                |
|------------------------|--------------------------------------------------------------------------------------------|
| `UseLogger`            | Sets the logger (kept for symmetry with sibling factories; dialer itself does not log yet) |
| `UseDefaultLogger`     | Sets the logger to `slog.Default`                                                          |
| `UseDialer`            | Overrides the underlying `*net.Dialer` used to reach the proxy                             |
| `UseProxyTLSConfig`    | Overrides the `*tls.Config` for the TLS handshake to an `https://` proxy                   |

### Terminal

| Method   | Description                                                          |
|----------|----------------------------------------------------------------------|
| `Build`  | Assembles and returns the `DialContextFunc` (or `nil` for direct)    |

## Mode dispatch

| `cfg.Mode`           | `Build()` result                                                            |
|----------------------|-----------------------------------------------------------------------------|
| empty / `none`       | `(nil, nil)` — direct dial                                                  |
| `url`                | tunnel resolved from `cfg.URL` scheme (`http`/`https`/`socks5`/`socks5h`)   |
| `host`               | http-proxy tunnel to `cfg.Host:cfg.Port` with optional `cfg.Auth`           |
| anything else        | error at `Build()` time                                                     |

See the [Proxy guide](../../../docs/proxy.md) for YAML modes and operator
guidance.
