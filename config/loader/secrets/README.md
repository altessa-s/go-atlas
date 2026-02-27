# secrets

```go
import "github.com/altessa-s/go-atlas/config/loader/secrets"
```

Package `secrets` provides secret placeholder expansion for the configuration loader. It replaces
`$__secret{namespace:key}` placeholders in struct fields and environment variables with actual secret values
retrieved from a `secrets.Manager`.

## Syntax

Placeholders use the format `$__secret{namespace:key}` where **namespace** is a logical grouping (e.g.,
`myapp`, `database`) and **key** is the specific secret identifier within the namespace.

## Options

| Option            | Description                                       |
|-------------------|---------------------------------------------------|
| `WithLogger`      | Set `*slog.Logger` for error reporting             |
| `WithFailOnError` | Fail-closed (default `true`) or fail-open on error |

## Error handling

By default the expander is **fail-closed**: if a secret cannot be resolved, `Expand` returns an error.
Set `WithFailOnError(false)` for fail-open behavior where unresolved placeholders become empty strings.

## Convenience functions

| Function        | Description                                           |
|-----------------|-------------------------------------------------------|
| `HasSecrets`    | Check if a string contains `$__secret{}` placeholders |
| `ExpandString`  | One-shot expand with a temporary `Expander`           |
| `ExpandEnvValue`| Expand placeholders in an environment variable value  |
| `ExpandStruct`  | Walk a struct via reflection and expand all fields     |

## Supported field types

`ExpandStruct` handles `string`, `*string`, `[]string`, `map[K]string`, nested structs, pointers, slices,
and maps containing structs.

## Thread safety

`Expander` is safe for concurrent use from multiple goroutines.
