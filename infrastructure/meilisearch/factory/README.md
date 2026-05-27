# factory

```go
import "github.com/altessa-s/go-atlas/infrastructure/meilisearch/factory"
```

Configuration-driven builder for [data/meilisearch.Client](../../../data/meilisearch). `ClientBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

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
| `New(cfg)` | Creates a `ClientBuilder` for the given `*config.Meilisearch`         |

### Dependencies

| Method                       | Description                                                                        |
|------------------------------|------------------------------------------------------------------------------------|
| `UseLogger(l)`               | Sets the logger for the builder and the created client                             |
| `UseDefaultLogger()`         | Shortcut for `UseLogger(slog.Default())`                                            |
| `UseHealthCoordinator(c)`    | Registers the client's health check with the given `health.Coordinator`            |
| `UseHealthServiceName(n)`    | Overrides the registered service name (default: `"meilisearch"`)                   |

### Terminal

| Method        | Description                                                                                  |
|---------------|----------------------------------------------------------------------------------------------|
| `Build(ctx)`  | Assembles the `*meilisearch.Client`, runs the synchronous startup health probe within `ctx`  |

## Errors

| Sentinel             | Returned by | Cause                                            |
|----------------------|-------------|--------------------------------------------------|
| `ErrConfigRequired`  | `Build`     | `New` was called with a nil `*config.Meilisearch` |

`errors.Is(err, factory.ErrConfigRequired)` lets dynamic config pipelines (e.g. YAML loaders that may omit the Meilisearch block) branch on the
specific failure cause.

## TLS

When `cfg.TLS != nil`, the builder delegates TLS-config construction to [security/tlsutils/factory](../../../security/tlsutils/factory) and wraps
the result into an `*http.Client` that the SDK uses for every outbound call. CA pool assembly, mTLS client-cert loading, `SkipVerifyMode`
enforcement, and the `ATLAS_ALLOW_INSECURE_TLS` env-var gate all happen in the canonical place — this factory does not reimplement any of it.

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

## See also

- [data/meilisearch](../../../data/meilisearch) — the underlying client API surface and options
- [config.Meilisearch](../../../config/meilisearch.go) — YAML-facing config struct
- [security/tlsutils/factory](../../../security/tlsutils/factory) — TLS configuration helpers
