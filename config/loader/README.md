# loader

```go
import "github.com/altessa-s/go-atlas/config/loader"
```

Package `loader` provides multi-source configuration loading with pluggable backends, environment variable mapping,
default values, and secret expansion.

## Features

- Load from YAML/TOML files, environment variables, and struct `default` tags
- Pluggable backends via `backend.Backend` interface (YAML is the default)
- Nested env mapping with `__` delimiter (`GRPC__INTERCEPTORS__CACHE__ENABLE`)
- Variable substitution: `${VAR}` or `${VAR:default}`
- Inline `$VAR` expansion in env values with `$$` escape (see below)
- Secret expansion: `$__secret{namespace:key}` via `secrets.Manager`
- Automatic validation, normalization, and default application
- Thread-safe after initialization

## Loading order

1. Read configuration file(s) from the specified path (file or directory)
2. Apply `default` struct tags to empty fields
3. Override with environment variables (camelCase converted to SCREAMING_SNAKE_CASE)
4. Re-apply defaults to newly created nested structs
5. Expand `$__secret{ns:key}` placeholders via the secrets manager
6. Run `Validate()` if the struct implements `loader.Validator`
7. Run `Normalize()` if the struct implements `loader.Normalizer`

## Interfaces

| Interface    | Method        | Purpose                                  |
|--------------|---------------|------------------------------------------|
| `Validator`  | `Validate()`  | Validate configuration after loading     |
| `Defaulter`  | `Default()`   | Set programmatic defaults                |
| `Normalizer` | `Normalize()` | Transform values to canonical form       |

## Options

| Option                  | Description                                      |
|-------------------------|--------------------------------------------------|
| `WithPath`              | Path to config file or directory                 |
| `WithPathOnEnvKey`      | Resolve path from env variable with fallback     |
| `WithEnvPrefix`         | Prefix for environment variable lookup           |
| `WithEnvSectionDelimiter` | Nested struct delimiter (default `__`)         |
| `WithStructTag`         | Struct tag name for field mapping (default `yaml`)|
| `WithSkipEnv`           | Skip environment variable loading                |
| `WithSkipDefaults`      | Skip default value application                   |
| `WithSecretsManager`    | Enable `$__secret{}` expansion                   |

## Environment variable expansion in values

Values read from environment variables go through a `$VAR` expansion pass
before being assigned to config fields:

| Input          | Result                                       |
|----------------|----------------------------------------------|
| `$VAR`         | Value of `VAR` (empty if unset)              |
| `$$`           | Literal `$` (escape; second `$` is consumed) |
| `$$VAR`        | Literal `$VAR` (no expansion of `VAR`)       |
| Bare `$`       | Preserved as a literal                       |

The `${VAR}` form is handled by a separate substitution pass (used for
default-value templates). In strict mode (`WithStrict`), referencing an
undefined `$VAR` returns an error; the `$$` escape and bare `$` never
trigger lookups and are safe in strict mode.

## Sentinel errors

| Error             | Meaning                                    |
|-------------------|--------------------------------------------|
| `ErrBindDefaults` | Default value cannot be bound to a field   |
| `ErrBindEnv`      | Environment value cannot be bound          |
| `ErrDecode`       | File decoding failed                       |
