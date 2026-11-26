# factory

```go
import "github.com/altessa-s/go-atlas/factory/infrastructure/meilisearch"
```

Configuration-driven builder for [store/meilisearch.Client](../../../store/meilisearch). `ClientBuilder` uses deferred error accumulation — errors from
any step are collected and returned at `Build()` time.

## Quick Start

```go
client, err := factory.New(cfg.Meilisearch).
    UseLogger(logger).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
if err != nil {
    return err
}
defer client.Close()
```

## Methods

### Constructor

| Method     | Description                                                           |
|------------|-----------------------------------------------------------------------|
| `New(cfg)` | Creates a `ClientBuilder` for the given `*meilisearchconfig.Config`         |

### Dependencies

| Method                       | Description                                                                        |
|------------------------------|------------------------------------------------------------------------------------|
| `UseLogger(l)`               | Sets the logger for the builder and the created client                             |
| `UseDefaultLogger()`         | Shortcut for `UseLogger(slog.Default())`                                            |
| `UseHealthCoordinator(c)`    | Registers the client's health check with the given `health.Registrar`            |
| `UseHealthServiceName(n)`    | Overrides the registered service name (default: `"meilisearch"`)                   |
| `WithoutHealth()`            | Disables coordinator registration while retaining the startup probe                |

### Terminal

| Method        | Description                                                                                  |
|---------------|----------------------------------------------------------------------------------------------|
| `Build(ctx)`  | Assembles the `*ManagedClient`, runs the synchronous startup health probe within `ctx`  |

`ManagedClient.Close` releases the health registration and idle HTTP connections. Use `client.Client` with APIs that require a
`*meilisearch.Client`.

## Errors

| Sentinel             | Returned by | Cause                                            |
|----------------------|-------------|--------------------------------------------------|
| `ErrConfigRequired`  | `Build`     | `New` was called with a nil `*meilisearchconfig.Config` |

`errors.Is(err, factory.ErrConfigRequired)` lets dynamic config pipelines (e.g. YAML loaders that may omit the Meilisearch block) branch on the specific
failure cause.

## TLS

When `cfg.TLS != nil`, the builder requires an HTTPS base URL and delegates TLS-config construction to [factory/ext/tlsutils](../../ext/tlsutils).
The resulting TLS config applies to all requests through the pooled HTTP transport.

```yaml
meilisearch:
  host: https://meilisearch.example.com
  apiKey: $__secret{search:api_key}
  tls:
    caCerts:
      - /etc/ssl/internal-ca.pem
    serverName: meilisearch.example.com
```

## Health Coordinator

`UseHealthCoordinator` registers a `health.Checker` that pings Meilisearch on demand. The default service name is `"meilisearch"`; override it via
`UseHealthServiceName` when running two Meilisearch instances against the same coordinator (would otherwise collide).
`WithoutHealth` disables registration. Probe failures logged by the coordinator checker use a fixed category.

## See also

- [store/meilisearch](../../../store/meilisearch) — the underlying client API surface and options
- [meilisearchconfig.Config](../../../config/meilisearch/meilisearch.go) — YAML-facing config struct
- [factory/ext/tlsutils](../../ext/tlsutils) — TLS configuration helpers
