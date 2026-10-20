# loader

```go
import "github.com/altessa-s/go-atlas/config/loader"
```

Package `loader` provides multi-source configuration loading with pluggable backends, environment variable mapping, default values, and secret
expansion.

## Features

- Load from YAML/TOML files, environment variables, and struct `default` tags
- Pluggable backends via `backend.Backend` interface (YAML is the default)
- Nested env mapping with `__` delimiter (`GRPC__INTERCEPTORS__CACHE__ENABLED`)
- Variable substitution: `${VAR}` or `${VAR:default}`
- Inline `$VAR` expansion in env values with `$$` escape (see below)
- Secret expansion: `$__secret{namespace:key}` via `secrets.Manager`
- Automatic validation, normalization, and default application
- Unknown file keys are rejected: a misspelled `enable:` for `enabled:` fails the load instead of being dropped silently
- Thread-safe after initialization

## Environment variable names

Every path of the environment loader (plain fields, indexed slices, nested sections and map entries) names variables by one rule:

1. With a prefix (`WithEnvPrefix`), only variables carrying it are read; any other variable is ignored, and the prefix is stripped before the rest of
   the name is matched. The prefix is normalized by `appinfo.NormalizeEnvKeyPrefix` (trimmed, uppercased, `-`/`.` to `_`, one trailing `_`):
   `app`, `APP` and `APP_` all mean `APP_`; services built with `appinfo.EnvPrefix` pass `appinfo.EnvKeyPrefix()`.
2. A field holding a value (anything but a struct section) with an `env` tag is named by the tag alone: `env:"DB_HOST"` reads `APP_DB_HOST`, and the
   field's path name does not apply. A tag starting with `APP_` or with the prefix maps to the prefix followed by the rest of the tag. Entries of such
   a slice or map are `APP_<TAG>__<index|key>`, also when the tag contains the delimiter (`env:"DB__LABELS"` reads `APP_DB__LABELS__team`). A tag
   claims a variable only when the rest of its name addresses an entry of the field's type; when several tags do, the longest wins, and a variable no
   tag claims is matched by path names. A tag of a scalar field has no entries: `APP_DB_HOST__X` is not read through `env:"DB_HOST"`.
3. Any other field is named by its path: one name per level joined by `__` (`WithEnvSectionDelimiter`). A level answers to its struct tag name (`yaml`
   unless `WithStructTag` says otherwise) and to its Go field name, both in SCREAMING_SNAKE_CASE and matched exactly (lower-case names are not
   read). Embedded structs add no level. Slice indices and map keys are levels taken verbatim; `NAME[key]` is accepted for `NAME__key`, with the
   configured delimiter in place of `__`.
4. Inside slice elements and map values, where a fixed name cannot address one field, `env` tags are ignored and the path name applies.

For ``MongoDB *Mongo `yaml:"mongodb"` `` both `APP_MONGODB__DATABASE` and `APP_MONGO_DB__DATABASE` set `mongodb.database`. When both are set, the
struct tag spelling wins on every path (plain and nested fields, slice indexes, map entries). In general, of the variables that set one value, only
one is applied: the one with the fewest levels spelled by Go field name over its whole name (on both sides of a slice index), then the first in name
order. Equal indexes spelled differently (`HOSTS__0`, `HOSTS__00`) set one element. A variable an `env` tag names whole belongs to that field only:
with `env:"LIST__0"` on a string and `env:"LIST"` on a slice, `APP_LIST__0` sets the string. Both spellings are accepted because the struct tag and
the Go field name differ for such fields and services already use either.

`APP_` at the start of an `env` tag is a placeholder for the configured prefix: with `WithEnvPrefix("RADAR")`, ``AppName string `env:"APP_NAME"` ``
reads `RADAR_NAME`. To read `RADAR_APP_NAME` instead, tag the field `env:"RADAR_APP_NAME"`. A tag that is only `APP_` or the prefix names itself, and
`env:"-"` keeps a value field out of the environment (slice elements and map values excepted, where `env` tags are ignored).

An error about a variable names it and never quotes its value, which may be an expanded secret: `environment variable APP_PORT: failed to parse an
integer: strconv.ParseInt: invalid syntax: numeric parse failure`.

A slice index in a variable name must not be negative, and an index past the end of the slice may grow it to at most `DefaultMaxEnvSliceLen` (1024)
elements, set by `WithMaxEnvSliceLen`; an index inside a longer slice from a file is always accepted. Any other index fails the load with
`ErrEnvIndexOutOfRange` (wrapped by `ErrBindEnv`) instead of allocating without bound. Recursive types (`Next *Node` in `Node`) load: the environment
reaches nested nodes by their path (`APP_HEAD__NEXT__NEXT__NAME`), allocating them on the way, to any depth the variable name spells out.
`Normalize()` of nested pointer structs runs down to the first field whose type repeats on its path (`head.next`, not `head.next.next`). Pointer
cycles in the value passed to `Load` (`n.Next = n`) are followed once by defaults and secret expansion, and a recursive embedding
(`type Node struct{ *Node }`) is searched once per variable name, allocating embedded pointers only for a name that matches.

## Loading order

1. Read configuration file(s) from the specified path (file or directory), recording which values each file sets
2. Override with environment variables (named as described above), recording which values they assign — an empty variable assigns
   nothing to a bool, number, duration, slice or map, so its default still applies
3. Apply defaults in one pass over the configuration, its nested structs and the struct elements of maps and slices: `Default()` first, then `default`
   tags for the zero values nothing set explicitly; a nil pointer struct is allocated when it has a field to default, unless it is tagged `default:"-"`
   or a file set it to `null`. In strict mode an undefined `${VAR}` in a tag fails only for a field that needs the default
4. Expand `$__secret{ns:key}` placeholders via the secrets manager
5. Run `Validate()` if the struct implements `loader.Validator`
6. Run `Normalize()` if the struct implements `loader.Normalizer`

Which values a file sets is decided the way the backend binds them: map keys are decoded into the map's key type (YAML `0x10` is `16` in a `map[int]T`
but stays `"0x10"` in a `map[string]T`), YAML `<<` merges follow yaml.v3's precedence, anonymous structs follow each backend's embedding rules (YAML
flattens only `,inline`; TOML flattens embedded structs without a tag name), and TOML keys match field names exactly before case-insensitively. A later
file overlays an earlier one the same way the decoder does: map entries are replaced whole, struct fields merge one by one, and TOML refills a slice's
backing array in place.

`Default()` may fill or derive any field. Afterwards every value a file or the environment set explicitly is restored from an independent copy taken
before it ran, while the values around it — other fields of an element, other map entries — keep what `Default()` set. When indexed environment
variables grow a slice or create a map, a `default` tag fills only the holes the environment left — elements it added but did not set and that are still
zero, or keys missing from a map it created; existing entries from the value passed to `Load`, a file or `Default()` are never overwritten.

## Behavior changes

Compared with earlier releases (breaking changes first):

A variable that names no field is ignored silently, even with `WithStrict`, so a renamed override simply stops applying: audit deployment manifests
for the names the items below no longer read.

- The env prefix is normalized by `appinfo.NormalizeEnvKeyPrefix`, the same rule `appinfo` uses: trimmed, uppercased, `-` and `.` replaced by `_`,
  exactly one trailing `_`. `WithEnvPrefix("app")`, `("APP")` and `("APP_")` all read `APP_*`; before, `APP` and `APP_` selected different names. A
  service whose prefix comes from `appinfo.EnvPrefix` passes `appinfo.EnvKeyPrefix()`.
- With a prefix set, only variables carrying it are read: unprefixed variables such as `SECRETS__MASTER_KEY` no longer override the configuration.
  Rename them to `APP_SECRETS__MASTER_KEY`.
- A level answers to its struct tag name and to its Go field name on every path (plain fields, indexed slices, nested sections, map entries):
  `APP_MAXCONNS` now also sets ``MaxConns int `yaml:"maxconns"` ``; before, plain fields answered to Go field names only (`APP_MAX_CONNS`), while
  nested paths also matched struct tags and `env` tags, case-insensitively.
- An `env` tag names its field only: it is not a level name of a section (`APP_<TAG>__FIELD` does not reach the section's fields), and a tagged field
  is no longer read from its path name. Use the tag variable, or drop the tag to use the path.
- Level names match only in SCREAMING_SNAKE_CASE: `APP_mongodb__database` is no longer read.
- An indexed variable whose index is negative or would grow a slice past `WithMaxEnvSliceLen` (default 1024) fails the load with
  `ErrEnvIndexOutOfRange`; before, it panicked or allocated the whole slice.
- `NAME[key]` follows `WithEnvSectionDelimiter`; before, it always became `NAME__key`.
- Configurations with recursive pointer types load; before, building the field list recursed until the process ran out of memory.
- When a value is set in both the struct tag and the Go field name spelling, the struct tag spelling wins; before, which one won was unspecified. Of
  several variables setting one value, only one is applied, and a variable an `env` tag names whole is not read as an entry of another tag.
- `env:"-"` keeps a value field out of the environment, and a tag that is only `APP_` or the prefix names itself (`env:"APP_"` reads `APP_APP_`).
- Parse errors name the variable and no longer quote the value (`ErrBindEnv` and `ErrBindDefaults` messages changed; the sentinels did not).
- The environment is applied before `Default()`, so `Default()` sees environment values; it runs once per eligible struct (the root, pointer structs and
  collection elements, not plain value-struct fields) instead of twice.
- An explicit `false`, `0` or `""` from a file or the environment now wins over a `default` tag; an empty environment variable is not an explicit value
  for a bool, number, duration, slice or map.
- Omitted, untagged pointer structs are allocated and defaulted at any depth, including inside map and slice elements; tag such a field `default:"-"` to
  keep it `nil`. A pointer struct a file sets to `null` also stays `nil` inside collection elements.
- A `default` tag is only substituted when it is applied, so in strict mode an undefined `${VAR}` in an unused default no longer fails.
- A file key that binds to no field of the configuration struct fails the load with `ErrDecode` wrapping `ErrUnknownField`; before, it was ignored, so a
  typo silently left a setting at its default. The error names every unknown key of the file. Map entry keys stay free, though struct values in a map
  are checked; content under `any` (including `map[string]any`) and types with their own unmarshaler stays free-form, but a top-level key that only
  holds a YAML anchor (`base: &base`) or an `x-` extension block now counts as unknown. Fix the keys, or pass `WithAllowUnknownFields()` to restore the
  old behavior. Custom backends opt in by implementing `backend.StrictDecoder`; without it they decode as before.
- Backends implementing `backend.KeyDecoder` must return a `backend.KeyNode`; see the
  [migration note](backend/README.md#migrating-a-keydecoder-breaking-change).

## Interfaces

| Type         | Method        | Description                          |
|--------------|---------------|--------------------------------------|
| `Validator`  | `Validate()`  | Validate configuration after loading |
| `Defaulter`  | `Default()`   | Set programmatic defaults            |
| `Normalizer` | `Normalize()` | Transform values to canonical form   |

## Options

| Option                    | Description                                                                    |
|---------------------------|--------------------------------------------------------------------------------|
| `WithPath`                | Path to config file or directory                                               |
| `WithPathOnEnvKey`        | Resolve path from env variable with fallback                                   |
| `WithEnvPrefix`           | Read only variables with this prefix (`app`, `APP` and `APP_` all mean `APP_`) |
| `WithEnvSectionDelimiter` | Nested struct delimiter (default `__`)                                         |
| `WithStructTag`           | Struct tag name for field mapping (default `yaml`)                             |
| `WithSkipEnv`             | Skip environment variable loading                                              |
| `WithSkipDefaults`        | Skip default value application                                                 |
| `WithAllowUnknownFields`  | Ignore file keys that bind to no struct field instead of failing               |
| `WithMaxConfigBytes`      | Per-file read cap in bytes (default 16 MiB)                                    |
| `WithMaxEnvSliceLen`      | Max length an env index can grow a slice to (default 1024)                     |
| `WithStrict`              | Fail on undefined env vars and unassignable values                             |
| `WithSecretsManager`      | Enable `$__secret{}` expansion                                                 |

## Environment variable expansion in values

Values read from environment variables go through a `$VAR` expansion pass before being assigned to config fields:

| Input          | Result                                       |
|----------------|----------------------------------------------|
| `$VAR`         | Value of `VAR` (empty if unset)              |
| `$$`           | Literal `$` (escape; second `$` is consumed) |
| `$$VAR`        | Literal `$VAR` (no expansion of `VAR`)       |
| Bare `$`       | Preserved as a literal                       |

The `${VAR}` form is handled by a separate substitution pass (used for default-value templates). In strict mode (`WithStrict`), referencing an undefined
`$VAR` returns an error; the `$$` escape and bare `$` never trigger lookups and are safe in strict mode.

## Sentinel errors

| Error                   | Meaning                                                              |
|-------------------------|----------------------------------------------------------------------|
| `ErrBindDefaults`       | Default value cannot be bound to a field                             |
| `ErrBindEnv`            | Environment value cannot be bound                                    |
| `ErrEnvIndexOutOfRange` | Env slice index negative or past the limit (wrapped by `ErrBindEnv`) |
| `ErrDecode`             | File decoding failed                                                 |
| `ErrUnknownField`       | A file key binds to no struct field (wrapped by `ErrDecode`)         |
| `ErrPathOutsideRoot`    | A config directory entry resolves (via symlink) outside the root     |
| `ErrInvalidConfig`      | The configuration is not a struct pointer                            |
| `ErrUndefinedEnvVar`    | Strict mode: a referenced environment variable is undefined          |
