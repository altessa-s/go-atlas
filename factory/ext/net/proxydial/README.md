# factory

```go
import "github.com/altessa-s/go-atlas/factory/ext/net/proxydial"
```

Package `factory` provides a fluent builder that materializes a [`proxyconfig.Config`](../../../../config/proxy/proxy.go) into a
[`proxydial.DialContextFunc`](https://github.com/altessa-s/go-atlas-net/tree/develop/proxydial/proxydial.go), suitable for any client that needs
raw-TCP-through-proxy: SMTP (go-mail's `WithDialContextFunc`), SOAP, gRPC's `WithContextDialer`, or any custom protocol that does not go through
`net/http`.

HTTP and gRPC clients should instead use their respective factories' `ProxyOptions(cfg.Proxy)` functions, which map the config to client
options; the `DialerBuilder` targets consumers without an httpclient/grpcclient in the picture. Explicit proxy modes on the HTTP client work with its
SSRF protection on: the dial to the proxy is guarded (allow a private proxy network with `httpclient.WithSSRFAllowedCIDRs`) and the proxy resolves
targets.

## Quick start

```go
dial, err := factory.New(cfg.SMTP.Proxy).
    UseProxyTLSConfig(tlsCfg).
    Build()
if err != nil {
    return nil, fmt.Errorf("build smtp proxy dialer: %w", err)
}
if dial != nil {
    mailOpts = append(mailOpts, mail.WithDialContextFunc(dial))
}
```

A nil `cfg`, an empty `Mode`, or mode `none` yields `(nil, nil)` — caller treats that as "use a direct dial" and skips wiring a custom dialer
entirely.

`Build` and the client factories resolve and validate the config with `Config.URLValue`. A malformed URL, or one whose
credentials spill into the host or path (an unencoded `/` or `@` in the password, see `redacted.URL.HasAmbiguousUserInfo`), is rejected with an error
that quotes no part of the URL, before any DNS lookup or dial. A validated URL's host cannot carry credentials, so proxydial dial errors may name the
proxy `host:port`.

## Methods

### Constructor

| Method     | Description                                                            |
|------------|------------------------------------------------------------------------|
| `New(cfg)` | Creates a `DialerBuilder` for the given `*proxyconfig.Config` (nil ok)       |

### Dependencies

| Method                 | Description                                                                                |
|------------------------|--------------------------------------------------------------------------------------------|
| `UseDialer`            | Overrides the underlying `*net.Dialer` used to reach the proxy                             |
| `UseProxyTLSConfig`    | Overrides the `*tls.Config` for the TLS handshake to an `https://` proxy                   |

### Client option mappers

| Function                 | Description                                                                         |
|--------------------------|-------------------------------------------------------------------------------------|
| `factory/ext/net/httpclient.ProxyOptions(cfg)` | Maps the config to [`go-atlas-net/http/client`](https://github.com/altessa-s/go-atlas-net/tree/develop/http/client) options; nil or empty `Mode` yields none |
| `factory/transport/grpc/client.ProxyOptions(cfg)` | Maps the config to `transport/grpc/client` options; nil or empty `Mode` yields none |

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

See the [Proxy guide](../../../../docs/proxy.md) for YAML modes and operator guidance.
