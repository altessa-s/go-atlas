# Outbound Proxy Configuration

Configure outbound HTTP and gRPC egress through forward proxies — corporate egress gateways, SOCKS5 tunnels, and HTTPS proxies with self-signed certificates
— using the same YAML-driven model across every consumer in go-atlas.

## Table of contents

- [Overview](#overview)
- [Components](#components)
- [YAML reference](#yaml-reference)
- [Programmatic API](#programmatic-api)
- [Wiring patterns](#wiring-patterns)
- [TLS to the proxy](#tls-to-the-proxy)
- [SOCKS5 vs SOCKS5h](#socks5-vs-socks5h)
- [SSRF interaction](#ssrf-interaction)
- [Defaults and operator guidance](#defaults-and-operator-guidance)
- [Known limitations](#known-limitations)

---

## Overview

Proxy support lives in three layers:

1. **Config structs** — `config.HTTPProxy` and `config.GrpcProxy` define the YAML schema and materialize into option slices via `ClientOptions()`.
2. **Client options** — the `WithProxy*` family of functional options on `transport/http/client` and `transport/grpc/client` applies the options to the respective clients.
3. **Shared dialer** — `transport/proxydial` implements HTTP CONNECT (RFC 7231 §4.3.6), SOCKS5, and the `*tls.Config` merge rules used by both clients.

### Default behavior

**Passthrough is the default.** When the `proxy` block is omitted from YAML (or the receiver is `nil`):

- `transport/http/client` uses `http.ProxyFromEnvironment` — the
  `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` environment variables decide.
- `transport/grpc/client` uses grpc-go's built-in env lookup
  (`HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`).

An explicit configuration overrides the env-based default — including the `Mode: none` mode, which disables proxy resolution entirely.

---

## Components

### Config types

| Type | Purpose |
|------|---------|
| `config.HTTPProxy` | YAML-driven HTTP proxy config; translates to `[]httpclient.Option` |
| `config.HTTPProxyAuth` | Username + secret-redacted password for HTTP proxy |
| `config.HTTPProxyMode` | Mode enum: `none` / `url` / `host` (or empty = passthrough) |
| `config.GrpcProxy` | YAML-driven gRPC proxy config; translates to `[]grpcclient.Option` |
| `config.GrpcProxyAuth` | Username + secret-redacted password for gRPC proxy |
| `config.GrpcProxyMode` | Mode enum: `none` / `url` / `host` (or empty = passthrough) |

### HTTP client options

| Option | Description |
|--------|-------------|
| `WithProxy(host, port, auth)` | Route through `http://host:port` with optional `*url.Userinfo` |
| `WithProxyURL(u)` | Route through any proxy URL (`http` / `https` / `socks5` / `socks5h`) |
| `WithProxyFunc(f)` | Custom resolver matching `http.Transport.Proxy` signature |
| `WithoutProxy()` | Disable proxy resolution entirely, including env-var lookup |
| `WithProxyTLSConfig(cfg)` | Custom `*tls.Config` for the handshake to an `https://` proxy |
| `HTTPClientSetter` | Interface `SetHTTPClient(*http.Client)` for sub-components that adopt the parent's client |

### gRPC client options

| Option | Description |
|--------|-------------|
| `WithProxy(host, port, auth)` | Route through `http://host:port` with optional `*url.Userinfo` |
| `WithProxyURL(u)` | Route through any proxy URL (`http` / `https` / `socks5` / `socks5h`) |
| `WithProxyFunc(f)` | Custom resolver matching `http.Transport.Proxy` signature |
| `WithoutProxy()` | Disable proxy resolution including grpc-go's env lookup |
| `WithProxyTLSConfig(cfg)` | Custom `*tls.Config` for the handshake to an `https://` proxy |

### Shared dialer

Both transports agree on TCP-level options through a shared dialer that exposes HTTP CONNECT, SOCKS5, TLS-config merging, and a default `Dialer` with `DialTimeout` / `DialKeepAlive` of 30s each.

---

## YAML reference

Both `http_proxy.yaml` and `grpc_proxy.yaml` share the same schema:

```yaml
proxy:
  # <string> Proxy resolution mode. One of: none | url | host.
  # Omit (or leave empty) to keep the underlying transport's default —
  # the standard HTTP_PROXY/HTTPS_PROXY/NO_PROXY env vars.
  #   none — disable proxy entirely, including env-based lookup
  #   url  — route via a single proxy URL
  #   host — route via host + port (optionally auth for credentials)
  mode: url

  # <string> Full proxy URL. Required when mode is "url".
  # Must include an explicit port — port-less URLs are rejected at
  # config load.
  # Supported schemes: http, https, socks5, socks5h.
  url: http://proxy.corp.example:3128

  # <string> Proxy hostname. Required when mode is "host".
  host: proxy.corp.example

  # <int> Proxy port (1–65535). Required when mode is "host".
  port: 3128

  # <object> Proxy credentials. Optional even when mode is "host".
  # Used only when mode is "host".
  auth:
    username: svc-account
    # Use $__secret{...} to pull the password from the secrets manager.
    password: $__secret{vault:proxy-password}
```

### Mode semantics

| Mode | Effect |
|------|--------|
| *(empty)* | Passthrough — env vars decide |
| `none` | Disable proxy resolution (even env vars) |
| `url` | Send all traffic to `url` (must include port) |
| `host` | Build `http://host:port` from fields; `auth.password` supports secret expansion |

### Validation rules

The `Validate()` methods enforce:

- `Mode` ∈ `{"", "none", "url", "host"}`;
- URL mode requires a non-empty `url` with an explicit port and a supported
  scheme (`http` / `https` / `socks5` / `socks5h`);
- Host mode requires non-empty `host` and a port in `[1, 65535]`;
- **Mutual exclusion**: fields irrelevant to the current mode must be empty
  (e.g. setting `url` while `mode: host` fails validation — this catches YAML typos that would otherwise be silently dropped).

### `!include` pattern for reusable proxy blocks

The YAML loader supports `!include <file>`, so every consumer can reuse the same template. This is how
`config/templates/opa.yaml` wires proxy settings for GitLab and S3 policy sources:

```yaml
opa:
  gitlab:
    endpoint: https://gitlab.example.com
    token: $__secret{vault:gitlab-token}
    projectID: 42
    # Outbound HTTP proxy for GitLab API requests.
    !include http_proxy.yaml

  s3:
    bucket: my-policies-bucket
    region: us-east-1
    # Outbound HTTP proxy for S3 API requests.
    !include http_proxy.yaml
```

When the `!include` line stays commented, the consumer falls back to env-var passthrough.

---

## Programmatic API

### HTTP client

```go
import (
    "net/url"

    httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// Host/port with credentials (most common).
c := httpclient.New(
    httpclient.WithProxy("proxy.corp", 3128, url.UserPassword("svc", token)),
    httpclient.WithLogger(logger),
)

// Full URL (use for socks5 or https proxy).
u, _ := url.Parse("socks5://proxy.corp:1080")
c = httpclient.New(httpclient.WithProxyURL(u))

// Opt out of env-based proxy entirely.
c = httpclient.New(httpclient.WithoutProxy())

// Custom resolver — branch per-request.
c = httpclient.New(httpclient.WithProxyFunc(func(req *http.Request) (*url.URL, error) {
    if strings.HasSuffix(req.URL.Host, ".internal") {
        return nil, nil // no proxy
    }
    return corporateProxy, nil
}))
```

### gRPC client

```go
import (
    "context"
    "crypto/tls"
    "crypto/x509"

    grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

ctx := context.Background()

// Host/port proxy.
c, err := grpcclient.New(ctx, "service.example.com:443",
    grpcclient.WithProxy("proxy.corp", 3128, nil),
)

// HTTPS proxy with a self-signed CA.
pool := x509.NewCertPool()
pool.AppendCertsFromPEM(corpProxyCA)
c, err = grpcclient.New(ctx, "service.example.com:443",
    grpcclient.WithProxyURL(mustParse("https://proxy.corp:3128")),
    grpcclient.WithProxyTLSConfig(&tls.Config{RootCAs: pool}),
)

// Disable env-based proxy.
c, err = grpcclient.New(ctx, "service.example.com:443",
    grpcclient.WithoutProxy(),
)
```

### Non-HTTP consumers (SMTP, IMAP, raw TCP, custom protocols)

Protocols that do not go through `net/http` use the [`transport/proxydial`](../transport/proxydial) package via its fluent factory at
[`transport/proxydial/factory`](../transport/proxydial/factory). The factory takes a `*config.HTTPProxy` and returns a `DialContextFunc`
(`func(ctx, network, addr) (net.Conn, error)`) that any library accepting a custom dialer can consume.

```go
import (
    proxydialfactory "github.com/altessa-s/go-atlas/transport/proxydial/factory"
    "github.com/wneessen/go-mail"
)

// Build the dialer once, when the SMTP provider starts.
dialFunc, err := proxydialfactory.New(cfg.SMTP.Proxy).
    UseLogger(logger).
    Build()
if err != nil {
    return nil, fmt.Errorf("build smtp proxy dialer: %w", err)
}

mailOpts := []mail.Option{ /* host, auth, TLS policy, ... */ }
if dialFunc != nil {
    mailOpts = append(mailOpts, mail.WithDialContextFunc(dialFunc))
}
client, err := mail.NewClient(cfg.SMTP.Host, mailOpts...)
```

A nil result means proxying is opted out of — caller should leave the
library on its default direct dialer. Concrete behaviour by `Mode`:

| Mode             | Build() result                                                              |
|------------------|-----------------------------------------------------------------------------|
| empty / `none`   | `nil` — direct dial                                                         |
| `url`            | tunnel resolved from `cfg.URL` scheme (`http`/`https`/`socks5`/`socks5h`)   |
| `host`           | http-proxy tunnel to `cfg.Host:cfg.Port` with optional `cfg.Auth`           |

The builder also exposes `UseDialer` (custom `*net.Dialer`) and `UseProxyTLSConfig` (TLS to the proxy itself) for callers that need to
override the defaults.

For consumers without an `HTTPProxy` config (proxy comes from env, runtime override, custom resolver), use `proxydial.FromURL` directly:

```go
proxyURL, _ := url.Parse(os.Getenv("HTTPS_PROXY"))
dialFunc, err := proxydial.FromURL(proxyURL,
    proxydial.WithDialer(&net.Dialer{Timeout: 5 * time.Second}),
)
```

Real-world fits: SMTP (`go-mail`), IMAP/POP3, LDAP, AMQP, Kafka, NATS, MQTT, Redis, MongoDB, PostgreSQL/MySQL drivers — every library that exposes a custom
dialer hook accepts a `DialContextFunc` produced by `proxydial`.

---

## Wiring patterns

Every consumer that supports proxy configuration follows the same pattern: materialize the config via `ClientOptions()` and pass the options to the target
client's constructor.

### OIDC

`auth/oidc/factory/builder.go`:

```go
proxyOpts, err := cfg.Proxy.ClientOptions()
if err != nil {
    return nil, b.WrapError(err, "materialize oidc proxy options")
}
if len(proxyOpts) > 0 {
    opts = append(opts, oidc.WithHTTPClientOptions(proxyOpts...))
}
```

The Provider's HTTP client is shared with any revocation loader that implements `httpclient.HTTPClientSetter` — so
discovery, JWKS refresh, introspection, userinfo, and URL-based revocation all use the same connection pool and proxy.

### OPA — GitLab source

`auth/opa/factory/builder.go`:

```go
proxyOpts, err := gl.Proxy.ClientOptions()
if err != nil {
    return nil, b.WrapError(err, "materialize gitlab proxy options")
}
if len(proxyOpts) > 0 {
    opts = append(opts, gitlab.WithHTTPClientOptions(proxyOpts...))
}
```

### OPA — S3 source

S3 is the one consumer that **conditionally injects** the resilient HTTP client — only when proxy is explicitly configured. Without an override the AWS SDK
keeps its own HTTP client (which already honors `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`) and its own retry layer, avoiding double-retry with go-atlas's
breaker:

```go
proxyOpts, err := s3Cfg.Proxy.ClientOptions()
if err != nil {
    return nil, b.WrapError(err, "materialize s3 proxy options")
}
if len(proxyOpts) > 0 {
    awsCfgOpts = append(awsCfgOpts, awsconfig.WithHTTPClient(httpclient.New(proxyOpts...)))
}
```

### OTLP — gRPC exporter

`observability/tracing/adapters/otlp/grpc_client.go`:

```go
opts := []grpcclient.Option{}
opts = slices.AppendIf(opts, c.insecure, grpcclient.WithInsecure())
if c.retry { /* ... */ }
// Caller-supplied options come last so they win over defaults.
// Used by the tracing factory to inject proxy resolvers from config.GrpcProxy.
opts = append(opts, c.extraClientOptions...)

client, err := grpcclient.New(ctx, c.endpoint, opts...)
```

---

## TLS to the proxy

Use `WithProxyTLSConfig(*tls.Config)` when the client must connect to an `https://` proxy that presents a certificate outside the system root store:

- Corporate proxy fronted by a self-signed CA.
- Proxy requiring client-certificate (mTLS) authentication.
- Pinning a specific minimum TLS version for the proxy leg only.

### Merge semantics

The caller's `*tls.Config` is **cloned** before use and never mutated. Two fields get safe defaults filled in **only when left at their zero value**:

- `ServerName` ← proxy URL's hostname (so SNI/verification targets the
  proxy, not the upstream);
- `MinVersion` ← `tls.VersionTLS12`.

Every other field (`RootCAs`, `Certificates`, `InsecureSkipVerify`, …) is preserved verbatim.

### When it's ignored

`WithProxyTLSConfig` has no effect for:

- plain `http://` proxies (no TLS handshake to the proxy);
- `socks5://` / `socks5h://` proxies (the SOCKS handshake is not TLS).

---

## SOCKS5 vs SOCKS5h

Both schemes are accepted by validation, but they behave **identically** in this implementation: `golang.org/x/net/proxy.SOCKS5` always sends
`ATYP=DomainName` for hostname addresses, so DNS resolution happens on the proxy side regardless of scheme.

Callers that genuinely need client-side DNS (for example to bypass a proxy's split-horizon resolver) must pre-resolve to an IP literal before handing the
address to the client.

---

## SSRF interaction

When `WithSSRFProtection()` is combined with any of the `WithProxy*` options, the dialed address becomes **the proxy itself** (not the ultimate
destination). If the proxy lives on a private network (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, …) the SSRF check will reject it.

Add the proxy's CIDR to the allowlist:

```go
httpclient.New(
    httpclient.WithProxyURL(corporateProxy),
    httpclient.WithSSRFProtection(),
    httpclient.WithSSRFAllowedCIDRs(
        netip.MustParsePrefix("10.0.0.0/8"),
    ),
)
```

---

## Defaults and operator guidance

### Dialer defaults

Both transports share `proxydial.DefaultDialer()` with:

| Field | Value |
|-------|-------|
| `Timeout` | 30s |
| `KeepAlive` | 30s |

These are exported as `proxydial.DefaultDialTimeout` and `proxydial.DefaultDialKeepAlive` so tests and alternative transports can reuse them without
duplicating literals.

### Port is mandatory

URL-mode proxies must include an explicit port. The YAML validator rejects `http://proxy.corp` with a clear error — stdlib's implicit port map (80/443/1080)
is bypassed deliberately because the custom dial path does not use it. Operators see the error at config load, not at every dial.

### Prefer `Mode: host` with `auth` block for credentials

Putting credentials into `url: http://user:pass@proxy:3128` works, but secrets are harder to mask in logs and harder to rotate independently of the URL. The
`host` / `port` / `auth` form with `password: $__secret{...}` keeps the secret out of plain YAML and gets redacted in `fmt` / `JSON` / `YAML` / `slog`
output automatically.

---

## Known limitations

- **OTLP `Protocol: http`**: `TracingOTLP.Proxy` is a `*GrpcProxy` and only
  wires into the gRPC exporter path. The config validator rejects the combination `Protocol: http` + `Proxy: {...}` at load time — the HTTP exporter must
  use env-var proxy (`HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`) instead. Full HTTP-exporter wiring would require separate
  `otlp.WithHTTPClientOptions([]otlptracehttp.Option)` plumbing and is tracked as a future enhancement.
- **OPA — S3**: the AWS SDK has its own retry layer, so go-atlas's resilient
  client is injected only when proxy is explicitly configured. If you need breaker / circuit-breaker semantics on S3 traffic without also setting a proxy,
  open an issue — the current design trades that off for single-retry correctness.
- **Client-side DNS for SOCKS5**: not available — see
  [SOCKS5 vs SOCKS5h](#socks5-vs-socks5h).

## See also

- [Configuration guide](configuration.md) — loader, env vars, templates
- [Architecture](architecture.md) — package layout, layering
- [OIDC](oidc.md) — `oidc.proxy` field, JWKS / introspection clients
- `transport/http/client` — HTTP client API reference
- `transport/grpc/client` — gRPC client API reference