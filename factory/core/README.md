# factory

```go
import "github.com/altessa-s/go-atlas/factory/core"
```

Package `factory` provides a base type and helpers for factory pattern implementations. Embed `Base` in concrete factories to get structured logging,
dependency validation, and standardized error formatting out of the box without any boilerplate.

## Base

| Function / Method        | Description                                              |
|--------------------------|----------------------------------------------------------|
| `NewBase`                | Create `Base` with logger (nil-safe, uses discard)       |
| `Logger`                 | Return the configured `*slog.Logger`                     |
| `SetLogger`              | Replace the logger (nil is ignored)                      |
| `RequireDependency`      | `ErrDependencyRequired` if a single dependency is nil    |
| `RequireAllDependencies` | Aggregate all nil entries of a `map[string]any` into one error via `errors.Join` |
| `Errorf`                 | Package-level: formatted error classified under a sentinel, message text unchanged |
| `WrapError`              | Wrap error with `%w` for `errors.Is`/`errors.As` support |
| `JoinErrors`             | Package-level: nil for empty slice, the sole error as-is, else `errors.Join`     |

## Errors

Builders across `factory/` classify their failures with the sentinels below. Match them with `errors.Is`; the message text names the offending
item and is not a stable contract.

| Sentinel                | Meaning                                                                                   |
|-------------------------|-------------------------------------------------------------------------------------------|
| `ErrConfigRequired`     | The builder got no configuration, or the section its selected backend needs is missing    |
| `ErrDependencyRequired` | A dependency the configuration calls for (client, connection, database, sink) is missing  |
| `ErrNotRegistered`      | The configuration names a known backend whose builder was not registered on the factory   |
| `ErrUnsupported`        | The configuration names a type, provider or backend the factory does not know             |

A nil builder passed to a `UseXxx` registration method is reported as `ErrInvalidArgument` of
[`go-atlas-core/errors`](https://github.com/altessa-s/go-atlas-core/tree/develop/errors). Backend factories keep their own narrower sentinels
(`ErrClientRequired`, `ErrConfigRequired`, ...) for the dependencies they require.

```go
storage, err := cachefactory.New(cfg).Build(ctx)
if errors.Is(err, corefactory.ErrNotRegistered) {
    // add the backend factory and register it with UseStorage
}
```
