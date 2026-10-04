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

1. Read configuration file(s) from the specified path (file or directory), recording which values each file sets
2. Record which values the environment sets, by loading it into a scratch copy of the configuration
3. Run `Default()` and apply `default` struct tags, skipping every value a file or the environment set — an explicit `false`, `0` or `""`
   is kept, and in strict mode an undefined `${VAR}` in a tag fails only for a field that needs the default
4. Override with environment variables (camelCase converted to SCREAMING_SNAKE_CASE)
5. Re-apply defaults to newly created nested structs, then to struct elements of maps and slices: `Default()` first, then `default` tags;
   a nil pointer struct is allocated unless it is tagged `default:"-"` or a file set it to `null`
6. Re-apply the environment
7. Expand `$__secret{ns:key}` placeholders via the secrets manager
8. Run `Validate()` if the struct implements `loader.Validator`
9. Run `Normalize()` if the struct implements `loader.Normalizer`

Which values a file sets is decided the way the backend binds them: map keys are decoded into the map's key type (YAML `0x10` is `16`
in a `map[int]T` but stays `"0x10"` in a `map[string]T`), YAML `<<` merges follow yaml.v3's precedence, anonymous structs follow each
backend's embedding rules (YAML flattens only `,inline`; TOML flattens embedded structs without a tag name), and TOML keys match field
names exactly before case-insensitively. A later file overlays an earlier one the same way the decoder does: map entries are replaced
whole, struct fields merge one by one.

`Default()` may fill or derive any field, but values a file or the environment set explicitly are restored after it runs.

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
