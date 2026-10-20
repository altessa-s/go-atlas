# Configuration

```go
import "github.com/altessa-s/go-atlas/config/loader"
```

Multi-source configuration loading with environment variable expansion, secret injection, and validation.

---

## Overview

The loader merges configuration from multiple sources in order of precedence (later sources override earlier):

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

| Step | Action                                                                            | Skip with            |
|------|-----------------------------------------------------------------------------------|----------------------|
| 1    | Read and decode configuration files                                               | No path configured   |
| 2    | Load environment variables                                                        | `WithSkipEnv()`      |
| 3    | Call `Default()` and apply `default` struct tags to fields nothing set (one pass) | `WithSkipDefaults()` |
| 4    | Expand `$__secret{namespace:key}` placeholders                                    | No secrets manager   |
| 5    | Call `Validate()` if implemented                                                  | --                   |
| 6    | Call `Normalize()` if implemented                                                 | --                   |

---

## Constructor options

```go
p := loader.New(backend,
    loader.WithPath("config.yaml"),
    loader.WithPathOnEnvKey("CONFIG_PATH", "config.yaml"),
    loader.WithEnvPrefix("APP"), // or appinfo.EnvKeyPrefix()
    loader.WithEnvSectionDelimiter("__"),
    loader.WithStructTag("yaml"),
    loader.WithSkipEnv(),
    loader.WithSkipDefaults(),
    loader.WithSecretsManager(manager),
    loader.WithStrict(),
    loader.WithMaxConfigBytes(16 * 1024 * 1024),
    loader.WithMaxEnvSliceLen(1024),
)
```

| Option                    | Default              | Description                                                                                                                                       |
|---------------------------|----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `WithPath`                | --                   | Config file path. Supports `~` home directory expansion                                                                                           |
| `WithPathOnEnvKey`        | --                   | Load path from env var with a fallback default path                                                                                               |
| `WithEnvPrefix`           | `""`                 | Read only variables with this prefix; normalized to one trailing `_` (`app`, `APP`, `APP_` all mean `APP_`)                                       |
| `WithEnvSectionDelimiter` | `"__"`               | Delimiter for nested struct mapping in env vars                                                                                                   |
| `WithStructTag`           | `"yaml"`             | Struct tag name for field mapping                                                                                                                 |
| `WithSkipEnv`             | `false`              | Skip environment variable loading                                                                                                                 |
| `WithSkipDefaults`        | `false`              | Skip default value application                                                                                                                    |
| `WithSecretsManager`      | `nil`                | Secrets manager for `$__secret{}` expansion                                                                                                       |
| `WithStrict`              | `false`              | Error on undefined env vars and unsupported field types                                                                                           |
| `WithMaxConfigBytes`      | `16 MiB`             | Cap per-file read size. Guards against `/dev/zero`, multi-GB tmpfs files, or malicious include targets that would OOM the loader before parsing   |
| `WithMaxEnvSliceLen`      | `1024`               | Max length an indexed env variable can grow a slice to; a larger or negative index fails with `ErrEnvIndexOutOfRange`                             |

---

## Configuration files

### Variable substitution

Use `${VAR}` (or `$VAR`) syntax in YAML/TOML files. Substitution happens at file read time, before parsing. There is no inline default:
an undefined variable expands to an empty string (to `ErrUndefinedEnvVar` in strict mode). A substituted key is still present in the file, so
its `default` tag does **not** apply — `host: ${DB_HOST}` with `DB_HOST` unset yields an explicit null, not `localhost`. Reserve substitution
for required variables (ideally with `WithStrict`); for optional overrides with a fallback, omit the key from the file and use `env` plus
`default` struct tags.

```yaml
database:
  host: ${DB_HOST}
  port: ${DB_PORT}
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

When the path points to a directory, all files are loaded and merged in alphabetical order. CRC32 checksums are computed for change detection.

The loader confines the directory walk to the configured root: any entry whose target — after `EvalSymlinks` — resolves outside the root is rejected
with `loader.ErrPathOutsideRoot`. Without this check a writer with access to the config volume could drop e.g. `creds.yaml -> /etc/shadow` and the
loader would happily decode whatever the link points at. In-root symlinks (the Kubernetes ConfigMap pattern, where files are exposed via
`..data/foo.yaml` → `..2026_04_25_…/foo.yaml`) keep working unchanged.

---

## Environment variables

Every field has an environment variable name; all paths of the loader (plain fields, indexed slices, nested sections, map entries) use one rule:

1. With `WithEnvPrefix("APP")` (or `"app"` or `"APP_"`, the same: the prefix is normalized by `appinfo.NormalizeEnvKeyPrefix`), only `APP_*`
   variables are read; anything else is ignored, and `APP_` is stripped before matching. Services built with `appinfo.EnvPrefix` pass
   `appinfo.EnvKeyPrefix()`.
2. A value field (not a struct section) with an `env` tag is named by the tag alone; its path name does not apply. A tag starting with `APP_` or with
   the prefix maps to the prefix plus the rest of the tag. Entries of such a slice or map are `<TAG>__<index|key>`, also when the tag contains the
   delimiter (`env:"DB__LABELS"` reads `APP_DB__LABELS__team`). A tag claims a variable only when the rest of its name addresses an entry of the
   field's type; when several tags do, the longest wins, and a variable no tag claims is matched by path names. A scalar's tag has no entries.
3. Any other field is named by its path: one name per level joined by the section delimiter (default `__`). A level answers to its struct tag name
   and to its Go field name, both in SCREAMING_SNAKE_CASE and matched exactly. Embedded structs add no level; slice indices and map keys are levels
   taken verbatim, and `NAME[key]` is read as `NAME`, the delimiter and `key`.
4. Inside slice elements and map values `env` tags are ignored and the path name applies.

```go
type Config struct {
    Host     string            `yaml:"host" env:"SERVICE_HOST"` // APP_SERVICE_HOST only
    Database Database          `yaml:"database"`                // APP_DATABASE__PORT
    MongoDB  Mongo             `yaml:"mongodb"`                 // APP_MONGODB__URI or APP_MONGO_DB__URI
    Replicas []string          `yaml:"replicas"`                // APP_REPLICAS__0
    Labels   map[string]string `yaml:"labels"`                  // APP_LABELS__team
}
```

When a level's struct tag and Go field name differ (`mongodb` and `MongoDB`), both spellings work. When both are set, the struct tag spelling wins on
every path. Of the variables that set one value, only one is applied: the one with the fewest levels spelled by Go field name over its whole name,
then the first in name order; `HOSTS__0` and `HOSTS__00` set one element. A variable an `env` tag names whole belongs to that field only.

`APP_` at the start of an `env` tag stands for the configured prefix: with `WithEnvPrefix("RADAR")`, `env:"APP_NAME"` reads `RADAR_NAME`; tag the
field `env:"RADAR_APP_NAME"` to read `RADAR_APP_NAME`. `env:"-"` keeps a value field out of the environment. Errors name the variable and never quote
its value, which may be an expanded secret.

A slice index must not be negative, and an index past the end of the slice may grow it to at most 1024 elements (`WithMaxEnvSliceLen`); an index
inside a longer slice from a file is always accepted. Any other index fails the load with `ErrEnvIndexOutOfRange`, wrapped by `ErrBindEnv`.
Recursive types (`Next *Node` in `Node`) load, and the environment reaches nested nodes by their path (`APP_HEAD__NEXT__NEXT__NAME`), allocating
them on the way. Pointer cycles in the value passed to `Load` are followed once by defaults and secret expansion, and a recursive embedding
(`type Node struct{ *Node }`) allocates embedded pointers only for a variable name that matches.

A map of maps takes one level per key at any depth (`APP_LABELS__team__owner` for `map[string]map[string]string`), also through an `env` tag and
inside slice elements. A variable whose path leads to nothing settable sets nothing and allocates no nil section, map or element on the way; in strict
mode, a level no field answers to below a recognized root field fails the load with `ErrFieldNotFound` instead.

Environment variable values support recursive `$VAR` unwrapping (max depth 10) with circular reference detection.

### Migrating environment variable names

The rule above replaced per-path naming. Breaking changes:

| Before                                                                      | Now                                                                                     |
|-----------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|
| `WithEnvPrefix("APP")` and `WithEnvPrefix("APP_")` selected different names | Both, and `"app"`, read `APP_*`; pass `appinfo.EnvKeyPrefix()` with `appinfo.EnvPrefix` |
| With a prefix, unprefixed `SECTION__FIELD` still overrode the configuration | Only `APP_*` variables are read; rename them to `APP_SECTION__FIELD`                    |
| Plain fields answered to Go field names only                                | Every level answers to its struct tag name and its Go field name                        |
| A section's `env` tag worked as its level name on nested paths              | An `env` tag names its field only; use the path, or the tag of each field               |
| A tagged field was also read from its path name                             | A tagged field is read from its tag only                                                |
| Nested paths matched level names case-insensitively                         | Level names match only in SCREAMING_SNAKE_CASE                                          |
| A negative or huge slice index panicked or allocated the whole slice        | It fails the load with `ErrEnvIndexOutOfRange`                                          |
| `NAME[key]` always became `NAME__key`                                       | `NAME[key]` uses the configured section delimiter                                       |
| Map of maps entries were ignored after the outer map was created            | They are set, at any depth                                                              |
| With both spellings set, which one won was unspecified                      | The struct tag spelling wins                                                            |
| `env:"-"` named a variable `-`                                              | `env:"-"` keeps the field out of the environment                                        |
| Parse errors quoted the value                                               | Parse errors name the variable only                                                     |

A variable whose first level names no field is ignored silently, even with `WithStrict`, so a renamed override simply stops applying; below a recognized
root field, `WithStrict` fails the load with `ErrFieldNotFound` for a level no field answers to. Before upgrading, audit deployment manifests for
unprefixed overrides, path names of `env`-tagged fields (`APP_SECTION__FIELD` for a field tagged `env:"DB_HOST"`), section `env` tags used as level
names and lower-case names.

---

## Default values

Use the `default` struct tag:

```go
type Config struct {
    Host    string `yaml:"host" default:"localhost"`
    Port    int    `yaml:"port" default:"5432"`
    Timeout string `yaml:"timeout" default:"${DEFAULT_TIMEOUT}"`
}
```

Defaults support environment variable substitution; an unset variable yields an empty value. Defaults are applied only to fields that are still
zero and were not set explicitly by a file or the environment. A trailing `,skip_zero` (`default:"value,skip_zero"`) is accepted and ignored.

---

## Secret expansion

The `$__secret{namespace:key}` syntax injects secrets from external providers at load time.

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

Secret expansion traverses all string fields, `*string`, `[]string`, and `map[K]string` fields recursively. By default, expansion is fail-closed --
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

Configuration structs can implement the `Validator` interface for automatic validation after all sources are merged:

```go
type Validator interface {
    Validate() error
}
```

The `config` package provides a `ValidateStruct` helper built on ozzo-validation for declarative field validation:

```go
func (c *Config) Validate() error {
    return validationconfig.ValidateStruct(c,
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

Called on the root struct and on every non-nil nested pointer-to-struct field that implements the interface (struct values are not visited). In a
recursive type the walk stops at the first field whose type repeats on its path: for `Next *Node` in `Node`, `head.next` is normalized but
`head.next.next` is not.

---

## Strict mode

When enabled via `WithStrict()`, the loader returns errors instead of silently ignoring issues:

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

The `config/templates` package provides 30+ pre-built YAML templates for common components:

| Category       | Templates                                                                                     |
|----------------|-----------------------------------------------------------------------------------------------|
| Transport      | `http.yaml`, `grpc.yaml`, `broker.yaml` + middleware/interceptor dirs                         |
| Proxy          | `http_proxy.yaml`, `grpc_proxy.yaml` (reusable via `!include`)                                |
| Data           | `mongo.yaml`, `redis.yaml`, `nats.yaml`, `cache_storage.yaml`                                 |
| Security       | `auth.yaml`, `auth_oidc.yaml`, `opa.yaml`, `vault.yaml`, `secrets.yaml`, `tls-*.yaml`         |
| Observability  | `observability.yaml`, `health.yaml`, `logger.yaml` (pprof is configured in `http.yaml`)       |
| Services       | `scheduler.yaml`, `probabilistic_filter.yaml`, `idempotency.yaml`, `idempotency_storage.yaml` |
| Rate limiting  | `limiter_tokenbucket.yaml`, `limiter_budget.yaml`, `dlock.yaml`                               |
| Infrastructure | `node.yaml`, `retry.yaml`, `s3.yaml`                                                          |

`http_proxy.yaml` and `grpc_proxy.yaml` are shared across consumers (OIDC, OPA GitLab/S3 sources, OTLP tracing) via the `!include` directive. See the
[Proxy guide](proxy.md) for modes, wiring, and TLS-to-proxy semantics.
