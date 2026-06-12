# factory

```go
import "github.com/altessa-s/go-atlas/core/factory"
```

Package `factory` provides a base type and helpers for factory pattern implementations. Embed `Base` in concrete factories to get structured
logging, dependency validation, and standardized error formatting out of the box without any boilerplate.

## Base

| Function / Method        | Description                                              |
|--------------------------|----------------------------------------------------------|
| `NewBase`                | Create `Base` with logger (nil-safe, uses discard)       |
| `Logger`                 | Return the configured `*slog.Logger`                     |
| `RequireDependency`      | Error if a single dependency is nil                      |
| `RequireAllDependencies` | Aggregate all nil entries of a `map[string]any` into one error via `errors.Join` |
| `Errorf`                 | Formatted error (no wrapping)                            |
| `WrapError`              | Wrap error with `%w` for `errors.Is`/`errors.As` support |
| `JoinErrors`             | Package-level: nil for empty slice, the sole error as-is, else `errors.Join`     |
