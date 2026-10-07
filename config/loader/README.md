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
- Unknown file keys are rejected: a misspelled `enable:` for `enabled:` fails the load instead of being dropped silently
- Thread-safe after initialization

## Loading order

1. Read configuration file(s) from the specified path (file or directory), recording which values each file sets
2. Override with environment variables (camelCase converted to SCREAMING_SNAKE_CASE), recording which values they assign — an empty
   variable assigns nothing to a bool, number, duration, slice or map, so its default still applies
3. Apply defaults in one pass over the configuration, its nested structs and the struct elements of maps and slices: `Default()` first,
   then `default` tags for the zero values nothing set explicitly; a nil pointer struct is allocated when it has a field to default,
   unless it is tagged `default:"-"` or a file set it to `null`. In strict mode an undefined `${VAR}` in a tag fails only for a field
   that needs the default
4. Expand `$__secret{ns:key}` placeholders via the secrets manager
5. Run `Validate()` if the struct implements `loader.Validator`
6. Run `Normalize()` if the struct implements `loader.Normalizer`

Which values a file sets is decided the way the backend binds them: map keys are decoded into the map's key type (YAML `0x10` is `16`
in a `map[int]T` but stays `"0x10"` in a `map[string]T`), YAML `<<` merges follow yaml.v3's precedence, anonymous structs follow each
backend's embedding rules (YAML flattens only `,inline`; TOML flattens embedded structs without a tag name), and TOML keys match field
names exactly before case-insensitively. A later file overlays an earlier one the same way the decoder does: map entries are replaced
whole, struct fields merge one by one, and TOML refills a slice's backing array in place.

`Default()` may fill or derive any field. Afterwards every value a file or the environment set explicitly is restored from an
independent copy taken before it ran, while the values around it — other fields of an element, other map entries — keep what
`Default()` set. When indexed environment variables grow a slice or create a map, a `default` tag fills only the holes the environment
left — elements it added but did not set and that are still zero, or keys missing from a map it created; existing entries from the value
passed to `Load`, a file or `Default()` are never overwritten.

## Behavior changes

Compared with earlier releases:

- The environment is applied before `Default()`, so `Default()` sees environment values; it runs once per eligible struct (the root, pointer structs and
  collection elements, not plain value-struct fields) instead of twice.
- An explicit `false`, `0` or `""` from a file or the environment now wins over a `default` tag; an empty environment variable is not an
  explicit value for a bool, number, duration, slice or map.
- Omitted, untagged pointer structs are allocated and defaulted at any depth, including inside map and slice elements; tag such a field
  `default:"-"` to keep it `nil`. A pointer struct a file sets to `null` also stays `nil` inside collection elements.
- A `default` tag is only substituted when it is applied, so in strict mode an undefined `${VAR}` in an unused default no longer fails.
- A file key that binds to no field of the configuration struct fails the load with `ErrDecode` wrapping `ErrUnknownField`; before, it was ignored, so a
  typo silently left a setting at its default. The error names every unknown key of the file. Map entry keys stay free, though struct values in a map
  are checked; content under `any` (including `map[string]any`) and types with their own unmarshaler stays free-form, but a top-level key that only
  holds a YAML anchor (`base: &base`) or an `x-` extension block now counts as unknown. Fix the keys, or pass `WithAllowUnknownFields()` to restore the
  old behavior. Custom backends opt in by implementing `backend.StrictDecoder`; without it they decode as before.
- Backends implementing `backend.KeyDecoder` must return a `backend.KeyNode`; see the
  [migration note](backend/README.md#migrating-a-keydecoder-breaking-change).

## Interfaces

| Type                      | Description                                                      |
|---------------------------|------------------------------------------------------------------|
| `Validator`               | `Validate()`  | Validate configuration after loading             |
| `Defaulter`               | `Default()`   | Set programmatic defaults                        |
| `Normalizer`              | `Normalize()` | Transform values to canonical form               |

## Options

| Option                  | Description                                      |
|-------------------------|--------------------------------------------------|
| `WithPath`                | Path to config file or directory                                 |
| `WithPathOnEnvKey`        | Resolve path from env variable with fallback                     |
| `WithEnvPrefix`           | Prefix for environment variable lookup                           |
| `WithEnvSectionDelimiter` | Nested struct delimiter (default `__`)                           |
| `WithStructTag`           | Struct tag name for field mapping (default `yaml`)               |
| `WithSkipEnv`             | Skip environment variable loading                                |
| `WithSkipDefaults`        | Skip default value application                                   |
| `WithAllowUnknownFields`  | Ignore file keys that bind to no struct field instead of failing |
| `WithStrict`              | Fail on undefined env vars and unassignable values               |
| `WithSecretsManager`      | Enable `$__secret{}` expansion                                   |

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
| `ErrBindDefaults`         | Default value cannot be bound to a field                         |
| `ErrBindEnv`              | Environment value cannot be bound                                |
| `ErrDecode`               | File decoding failed                                             |
| `ErrUnknownField`         | A file key binds to no struct field (wrapped by `ErrDecode`)     |
