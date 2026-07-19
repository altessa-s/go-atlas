# proxydial

```go
import "github.com/altessa-s/go-atlas/transport/proxydial"
```

Forward-proxy dialers for client connections that do **not** go through `net/http`: SMTP, SOAP, raw TCP, gRPC, or any other protocol that needs
to tunnel through a corporate or egress proxy.

## When to use what

| Layer        | Symbol                                                | Use when…                                                                  |
|--------------|-------------------------------------------------------|----------------------------------------------------------------------------|
| **High**     | `factory.New(cfg).Use*().Build()`                     | Your service already loads `config.Proxy` from YAML/env                    |
| **Middle**   | `proxydial.FromURL(u, opts...)`                       | Proxy comes from a `*url.URL` (env, runtime override, custom resolver)     |
| **Low**      | `proxydial.HTTPConnect` / `proxydial.SOCKS5Dialer`    | One-shot persistent connection, custom retry policy, custom TLS handshake  |

## Quick example: SMTP via go-mail

```go
import proxydialfactory "github.com/altessa-s/go-atlas/transport/proxydial/factory"

dialFunc, err := proxydialfactory.New(cfg.Proxy).
    UseLogger(logger).
    Build()
if err != nil {
    return nil, fmt.Errorf("build smtp proxy dialer: %w", err)
}
if dialFunc != nil {
    mailOpts = append(mailOpts, mail.WithDialContextFunc(dialFunc))
}
client, err := mail.NewClient(host, mailOpts...)
```

## Quick example: SOAP via custom HTTP client

```go
proxyURL, _ := url.Parse(os.Getenv("HTTPS_PROXY"))
dial, err := proxydial.FromURL(proxyURL)
if err != nil { return err }

tr := &http.Transport{DialContext: dial}
client := &http.Client{Transport: tr}
```

## Wire flow by scheme

| Scheme              | Wire flow                                                               |
|---------------------|-------------------------------------------------------------------------|
| `http://`           | TCP + HTTP `CONNECT`                                                    |
| `https://`          | TCP + TLS-to-proxy + HTTP `CONNECT`                                     |
| `socks5/socks5h`    | `golang.org/x/net/proxy.SOCKS5` (TCP, no TLS to the proxy)              |

## Symbols

| Symbol                                    | Description                                                                                    |
|-------------------------------------------|------------------------------------------------------------------------------------------------|
| `factory.DialerBuilder`                   | High-level — fluent builder that folds a `config.Proxy` into a `DialContextFunc`               |
| `FromURL`                                 | Middle-level — build a `DialContextFunc` from a parsed `*url.URL`                              |
| `HTTPConnect`                             | Low-level — open TCP+TLS to the proxy and send a `CONNECT` for `addr`                          |
| `SOCKS5Dialer`                            | Low-level — construct a SOCKS5 dialer for `proxyURL`                                           |
| `TLSConfig`                               | Clone the user's `*tls.Config` and fill `ServerName`/`MinVersion` from the URL when zero       |
| `WithDialer`                              | Override the underlying `*net.Dialer` for `FromURL`                                            |
| `WithProxyTLSConfig`                      | Override the `*tls.Config` for the TLS handshake to an `https://` proxy                        |
| `DefaultDialer`                           | `*net.Dialer` pre-configured with `DefaultDialTimeout` and `DefaultDialKeepAlive`              |
| `DefaultDialTimeout`                      | Default TCP connect timeout (30s)                                                              |
| `DefaultDialKeepAlive`                    | Default TCP keep-alive interval (30s)                                                          |
| `DialContextFunc`                         | Type alias for `func(ctx, network, addr) (net.Conn, error)` (matches `net.Dialer.DialContext`) |

## Out of scope

- End-to-end testing against a real HTTPS proxy with custom CA — consumers cover that at their integration layer.
- Proxy auto-detection (PAC, WPAD) — callers supply a static URL or a `http.ProxyFromEnvironment`-style resolver.
