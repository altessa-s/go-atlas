# factory

```go
import "github.com/altessa-s/go-atlas/auth/oidc/factory"
```

Package `factory` provides configuration-based creation of OIDC providers. Integrates with `config.OIDC` to create `Provider` instances with
validation options, presets, caching, introspection, and revocation support.

## Methods

| Method                              | Description                                                       |
|-------------------------------------|-------------------------------------------------------------------|
| `CreateProviderFromConfig`          | Create an OIDC provider with all options derived from config      |
| `CreateRevocationStorageFromConfig` | Create a revocation storage backed by probabilistic filters       |
| `ValidationOptionsFromConfig`       | Build validation options from config (used internally)            |
| `PresetFromConfig`                  | Build a validation preset from config (used internally)           |

## Options

| Option             | Default   | Description                                           |
|--------------------|-----------|-------------------------------------------------------|
| `WithScheduler`    | nil       | Task registrar for background JWKS refresh            |
| `WithLogger`       | discard   | Structured logger (`*slog.Logger`)                    |
| `WithTokenCache`   | nil       | Cacher implementation for validated token caching     |
| `WithRedisClient`  | nil       | Redis client for revocation filter storage            |
