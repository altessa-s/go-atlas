# Configuration

```go
import "github.com/altessa-s/go-atlas/config/loader"
```

Multi-source configuration loading with environment variable expansion,
secret injection, and validation.

---

## Overview

The loader merges configuration from multiple sources in order of precedence
(later sources override earlier):

1. **Default values** -- struct tags (`default:"value"`)
2. **Configuration files** -- YAML or TOML
3. **Environment variables** -- struct tags (`env:"VAR_NAME"`)
4. **Secret expansion** -- `$__secret{namespace:key}` syntax
5. **Validation** -- `Validate()` interface
6. **Normalization** -- `Normalize()` interface

---

## Quick start

```go
type Config struct {
    Database struct {
        Host string `yaml:"host" default:"localhost" env:"DB_HOST"`
        Port int    `yaml:"port" default:"5432" env:"DB_PORT"`
    } `yaml:"database"`
}

cfg := &Config{}
p := loader.New(&yaml3.Backend{}, loader.WithPath("config.yaml"))
p.Load(cfg)
```

---

## Loading pipeline

The `Load()` method executes the following steps in order:

| Step | Action                                         | Skip with            |
|------|------------------------------------------------|----------------------|
| 1    | Read and decode configuration files            | No path configured   |
| 2    | Apply `default` struct tags                    | `WithSkipDefaults()` |
| 3    | Load environment variables                     | `WithSkipEnv()`      |
| 4    | Re-apply defaults to newly created structs     | `WithSkipDefaults()` |
| 5    | Expand `$__secret{namespace:key}` placeholders | No secrets manager   |
| 6    | Call `Validate()` if implemented               | --                   |
| 7    | Call `Normalize()` if implemented              | --                   |

---

## Constructor options

```go
p := loader.New(backend,
    loader.WithPath("config.yaml"),
    loader.WithPathOnEnvKey("CONFIG_PATH", "config.yaml"),
    loader.WithEnvPrefix("APP"),
    loader.WithEnvDelimiter("_"),
    loader.WithEnvSectionDelimiter("__"),
    loader.WithStructTag("yaml"),
    loader.WithSkipEnv(),
    loader.WithSkipDefaults(),
    loader.WithSecretsManager(manager),
    loader.WithStrict(),
)
```

| Option                    | Default  | Description                                             |
|---------------------------|----------|---------------------------------------------------------|
| `WithPath`                | --       | Config file path. Supports `~` home directory expansion |
| `WithPathOnEnvKey`        | --       | Load path from env var with a fallback default path     |
| `WithEnvPrefix`           | `""`     | Prefix for all environment variables (e.g. `APP_`)      |
| `WithEnvDelimiter`        | `"_"`    | Delimiter for compound env var names                    |
| `WithEnvSectionDelimiter` | `"__"`   | Delimiter for nested struct mapping in env vars         |
| `WithStructTag`           | `"yaml"` | Struct tag name for field mapping                       |
| `WithSkipEnv`             | `false`  | Skip environment variable loading                       |
| `WithSkipDefaults`        | `false`  | Skip default value application                          |
| `WithSecretsManager`      | `nil`    | Secrets manager for `$__secret{}` expansion             |
| `WithStrict`              | `false`  | Error on undefined env vars and unsupported field types |

---

## Configuration files

### Variable substitution

Use `${VAR}` or `${VAR:default}` syntax in YAML/TOML files. Substitution happens at
file read time, before parsing.

```yaml
database:
  host: ${DB_HOST:localhost}
  port: ${DB_PORT:5432}
```

### YAML include directive

YAML files support `!include` for composing configurations from multiple files:

```yaml
database:
  !include database.yaml
logging:
  !include logging.yaml
```

- Max depth: 10 levels of nested includes
- Circular include detection
- Path traversal protection (only `.yaml` and `.yml` files allowed)
- Indentation-aware insertion

### Multi-file loading

When the path points to a directory, all files are loaded and merged in
alphabetical order. CRC32 checksums are computed for change detection.

---

## Environment variables

Environment variables are mapped to struct fields using the `env` tag:

```go
type Config struct {
    Host string `yaml:"host" env:"APP_HOST"`
    Port int    `yaml:"port" env:"APP_PORT"`
}
```

Nested structures use the section delimiter (default `__`):

```bash
export DATABASE__HOST=localhost
export DATABASE__PORT=5432
```

Environment variable values support recursive `$VAR` unwrapping (max depth 10)
with circular reference detection.

---

## Default values

Use the `default` struct tag:

```go
type Config struct {
    Host    string `yaml:"host" default:"localhost"`
    Port    int    `yaml:"port" default:"5432"`
    Timeout string `yaml:"timeout" default:"${DEFAULT_TIMEOUT:30s}"`
}
```

Defaults support environment variable substitution. The `skip_zero` modifier
prevents overwriting non-zero values: `default:"value",skip_zero`.

---

## Secret expansion

The `$__secret{namespace:key}` syntax injects secrets from external providers
at load time.

```yaml
database:
  password: $__secret{myapp:db_password}
  api_key: $__secret{external:api_key}
```

Configure the secrets manager:

```go
manager, _ := secrets.New[string](provider)
p := loader.New(nil,
    loader.WithPath("config.yaml"),
    loader.WithSecretsManager(manager),
)
```

Secret expansion traverses all string fields, `*string`, `[]string`, and
`map[K]string` fields recursively. By default, expansion is fail-closed --
missing secrets cause an error.

### Secret providers

| Provider             | Package                              | Best for                     |
|----------------------|--------------------------------------|------------------------------|
| HashiCorp Vault      | `security/secrets/providers/vault`   | Production secret management |
| GCP Secret Manager   | `security/secrets/providers/gcp`     | GCP environments             |
| Yandex Cloud Lockbox | `security/secrets/providers/lockbox` | Yandex Cloud environments    |
| In-memory            | `security/secrets/providers/memory`  | Testing, development         |

---

## Validation

Configuration structs can implement the `Validator` interface for automatic
validation after all sources are merged:

```go
type Validator interface {
    Validate() error
}
```

The `config` package provides a `ValidateStruct` helper built on ozzo-validation
for declarative field validation:

```go
func (c *Config) Validate() error {
    return config.ValidateStruct(c,
        validation.Field(&c.Port, validation.Required, validation.Min(1), validation.Max(65535)),
    )
}
```

---

## Normalization

Structs can implement `Normalizer` for post-validation transformations:

```go
type Normalizer interface {
    Normalize()
}
```

Called recursively on the root struct and all nested structs that implement the
interface.

---

## Strict mode

When enabled via `WithStrict()`, the loader returns errors instead of silently
ignoring issues:

| Error                     | Condition                                 |
|---------------------------|-------------------------------------------|
| `ErrUndefinedEnvVar`      | `${VAR}` references an undefined variable |
| `ErrUnsupportedFieldType` | Field type not handled by env loader      |
| `ErrFieldAssignment`      | Type incompatibility during assignment    |
| `ErrFieldNotFound`        | Mapped field does not exist in struct     |

---

## Backends

| Backend | Extensions          | Package                          |
|---------|---------------------|----------------------------------|
| YAML v3 | `.yaml`, `.yml`     | `config/loader/backend/yaml3`    |
| TOML    | `.toml`, `.tml`     | `config/loader/backend/toml`     |

---

## Configuration templates

The `config/templates` package provides 30+ pre-built YAML templates for common
components:

| Category       | Templates                                                                             |
|----------------|---------------------------------------------------------------------------------------|
| Transport      | `http.yaml`, `grpc.yaml`, `broker.yaml` + middleware/interceptor dirs                 |
| Proxy          | `http_proxy.yaml`, `grpc_proxy.yaml` (reusable via `!include`)                         |
| Data           | `mongo.yaml`, `redis.yaml`, `nats.yaml`, `cache_storage.yaml`                         |
| Security       | `auth.yaml`, `auth_oidc.yaml`, `opa.yaml`, `vault.yaml`, `secrets.yaml`, `tls-*.yaml` |
| Observability  | `observability.yaml`, `health.yaml`, `logger.yaml`, `pprof.yaml`                      |
| Services       | `scheduler.yaml`, `probabilistic_filter.yaml`, `idempotency.yaml`                     |
| Rate limiting  | `limiter_tokenbucket.yaml`, `limiter_budget.yaml`, `dlock.yaml`                       |
| Infrastructure | `node.yaml`, `retry.yaml`, `s3.yaml`                                                  |

`http_proxy.yaml` and `grpc_proxy.yaml` are shared across consumers (OIDC,
OPA GitLab/S3 sources, OTLP tracing) via the `!include` directive. See the
[Proxy guide](proxy.md) for modes, wiring, and TLS-to-proxy semantics.
