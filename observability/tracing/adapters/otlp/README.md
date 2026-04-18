# otlp

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"
```

Package `otlp` provides an OpenTelemetry Protocol adapter for the Atlas
tracing system. Exports spans over gRPC (`Protocol: grpc`, default) or HTTP
(`Protocol: http`) using the OTLP specification.

## Options

| Option                  | Default          | Description                                                                 |
|-------------------------|------------------|-----------------------------------------------------------------------------|
| `WithEndpoint`          | `localhost:4317` | OTLP collector endpoint                                                     |
| `WithProtocol`          | `grpc`           | Wire protocol — `grpc` or `http`                                            |
| `WithInsecure`          | false            | Skip TLS for the collector connection (gRPC only)                           |
| `WithCompression`       | false            | Enable gzip compression                                                     |
| `WithHeaders`           | --               | Static headers attached to every export request                             |
| `WithExportTimeout`     | 30s              | Per-export timeout                                                          |
| `WithServiceName`       | --               | OpenTelemetry service.name resource attribute                               |
| `WithServiceVersion`    | --               | OpenTelemetry service.version resource attribute                            |
| `WithEnvironment`       | --               | OpenTelemetry deployment.environment resource attribute                     |
| `WithResourceAttrs`     | --               | Additional resource attributes                                              |
| `WithRetry`             | disabled         | Enable retry with default config (gRPC only)                                |
| `WithRetryConfig`       | --               | Enable retry with custom `*grpcclient.RetryConfig` (gRPC only)              |
| `WithGRPCClientOptions` | --               | Forward `grpcclient.Option` values (proxy, TLS-to-proxy, dial options) to the underlying gRPC client (gRPC only) |

## Outbound proxy

For `Protocol: grpc`, route the OTLP collector connection through a forward
proxy by passing proxy options through `WithGRPCClientOptions`:

```go
import (
    "net/url"

    "github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"
    grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

proxyURL, _ := url.Parse("http://proxy.corp:3128")
adapter, err := otlp.New(
    otlp.WithEndpoint("collector.internal:4317"),
    otlp.WithGRPCClientOptions(
        grpcclient.WithProxyURL(proxyURL),
        // Optional: corp HTTPS proxy with self-signed CA
        // grpcclient.WithProxyTLSConfig(&tls.Config{RootCAs: pool}),
    ),
)
```

For YAML-driven configuration via `tracing.otlp.proxy`, see the
[Proxy guide](../../../../docs/proxy.md) and the
[tracing factory README](../../factory/).

`Protocol: http` does **not** support `WithGRPCClientOptions` proxy wiring.
The factory rejects the combination `Protocol: http + Proxy: {...}` at
config-load — operators must use `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` env
vars in that mode.
