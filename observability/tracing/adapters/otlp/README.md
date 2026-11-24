# otlp

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"
```

Package `otlp` provides an OpenTelemetry Protocol adapter for the Atlas tracing system. Exports spans over gRPC (`Protocol: grpc`, default) or HTTP
(`Protocol: http`) using the OTLP specification.

## Options

| Option                  | Default          | Description                                                                 |
|-------------------------|------------------|-----------------------------------------------------------------------------|
| `WithEndpoint`          | `localhost:4317` | OTLP collector endpoint                                                     |
| `WithProtocol`          | `grpc`           | Wire protocol — `grpc` or `http`                                            |
| `WithInsecure`          | false            | Disable TLS for the collector connection                                    |
| `WithCompression(bool)` | true             | Gzip compression; `false` disables it                                       |
| `WithHeaders`           | --               | Static headers attached to every export request                             |
| `WithExportTimeout`     | 30s              | Per-export timeout (gRPC only)                                              |
| `WithGRPCConn`          | --               | Caller-owned `grpc.ClientConnInterface` to export on (gRPC only); not closed by `Shutdown` |
| `WithHTTPClient`        | --               | `*http.Client` to export with (HTTP only)                                   |
| `WithServiceName`       | `unknown-service` | OpenTelemetry service.name resource attribute                               |
| `WithServiceVersion`    | --               | OpenTelemetry service.version resource attribute                            |
| `WithEnvironment`       | --               | OpenTelemetry deployment.environment resource attribute                     |
| `WithResourceAttrs`     | --               | Additional resource attributes                                              |

Each span carries the resource of the tracer that recorded it (`tracing.WithServiceName`, `WithServiceVersion`, `WithEnvironment`). The adapter
exports one resource per tracer: the span's resource attributes win, the adapter's attributes above fill the keys the tracer leaves unset or
empty, and spans without a resource use the adapter's resource as is.

## Outbound clients

The adapter sends on the client it is given. Without one it falls back to defaults so standalone use works: for gRPC it dials `WithEndpoint` with
grpc-go (TLS 1.2+ unless `WithInsecure`) and closes that connection on `Shutdown`; for HTTP it uses the `otlptracehttp` default client. Both
defaults honor `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`.

To apply a proxy, retry or egress policy, build the client with `transport/grpc/client` or
[`go-atlas-net/http/client`](https://github.com/altessa-s/go-atlas-net/tree/develop/http/client) and inject it:

```go
import (
    "net/url"

    "github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"
    grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

proxyURL, _ := url.Parse("http://proxy.corp:3128")
client, err := grpcclient.New(ctx, "collector.internal:4317", grpcclient.WithProxyURL(proxyURL))
if err != nil {
    return err
}
defer client.Close(ctx)
conn, err := client.GetConnection(ctx)
if err != nil {
    return err
}
adapter, err := otlp.New(ctx, otlp.WithEndpoint("collector.internal:4317"), otlp.WithGRPCConn(conn))
```

The [tracing factory](../../../../factory/observability/tracing/) does this for YAML-driven configuration (`tracing.adapters.otlp.proxy`) on both
protocols; see the [Proxy guide](../../../../docs/proxy.md).
