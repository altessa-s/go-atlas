# factory

```go
import "github.com/altessa-s/go-atlas/factory/observability/tracing"
```

Package `factory` provides a fluent builder for creating tracers from configuration. `TracerBuilder` uses deferred error accumulation — errors from any
step are collected and returned at `Build()` time. Returns `tracing.Noop` when config is `nil` or tracing is disabled.

## Quick Start

```go
tracer, err := factory.New(cfg.Tracing).
    UseAdapter(observabilityconfig.TracingTypeOTLP, otlpfactory.Builder()).
    UseLogger(logger).
    Build(ctx)
```

Console and noop adapters are built in. Any other backend registers its builder with `UseAdapter`; a configured type without one fails at
`Build(ctx)` with an error naming the module to add.

## Supported Adapter Types

| Type | Backend | Notes |
|------|---------|-------|
| `otlp` | OpenTelemetry (gRPC/HTTP) | Register [`factory/observability/tracing/adapters/otlp`](adapters/otlp)`.Builder()`; default endpoint `localhost:4317` |
| `console` | Stdout | Supports pretty-print and timestamps |
| `noop` | No-op | No spans emitted |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `TracerBuilder` for the given tracing config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseDefaultLogger` | Uses `slog.Default()` as the logger |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the tracer; returns `tracing.Noop` if disabled |

## Proxy wiring

For the `otlp` adapter, `otlpfactory.Builder()` builds the exporter's outbound client and injects it, so both protocols share one proxy policy from
`cfg.Adapters.OTLP.Proxy` (YAML `tracing.adapters.otlp.proxy`, a [`proxyconfig.Config`](../../../config/proxy/proxy.go)):

| Protocol | Client                                                                                                             | Injected with         |
|----------|--------------------------------------------------------------------------------------------------------------------|-----------------------|
| `grpc`   | `transport/grpc/client` with `grpcclientfactory.ProxyOptions(proxy)`                                     | `otlp.WithGRPCConn`   |
| `http`   | [`go-atlas-net/http/client`](https://github.com/altessa-s/go-atlas-net/tree/develop/http/client) with `httpclientfactory.ProxyOptions(proxy)`, an egress policy for the collector, no retry | `otlp.WithHTTPClient` |

A nil/empty `Proxy` block keeps the `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` env lookup on both paths. Retries stay with `otlptracehttp`, which applies
the OTLP retry rules. The adapter's `Shutdown` closes the gRPC client the builder made. See the [Proxy guide](../../../docs/proxy.md) for full mode
semantics.

## SSRF protection

The HTTP client keeps SSRF protection on. The collector is an operator-configured, often private endpoint, so its egress policy allows exactly the
configured `endpoint` host and port — a literal IP as that address, a host name at whatever address it resolves to when dialed — and the proxy that
carries the export (the configured one, or the environment proxy for the collector URL). Any other private, loopback, link-local or reserved
destination is refused with `httpclient.ErrSSRFBlocked`. Redirects are not followed at all: the policy admits any public host, so a redirect from a
compromised collector would otherwise replay the export, custom `headers` (API keys) included, to a host of its choosing; a 3xx reply fails the
export. A malformed `endpoint` (not `host[:port]`) fails `Build`.

The gRPC exporter dials only the configured endpoint (gRPC has no redirects), so it is not guarded.
