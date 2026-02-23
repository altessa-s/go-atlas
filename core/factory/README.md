# factory

```go
import "github.com/altessa-s/go-atlas/core/factory"
```

Package `factory` provides a base type and helpers for factory pattern implementations. Embed `Base` in concrete factories to get structured logging, dependency validation, and standardized error formatting.

## Usage

```go
type MyFactory struct {
    factory.Base
}

func New(logger *slog.Logger) *MyFactory {
    return &MyFactory{Base: factory.NewBase(logger)}
}
```

## Base

| Function / Method        | Description                                              |
|--------------------------|----------------------------------------------------------|
| `NewBase`                | Create `Base` with logger (nil-safe, uses discard)       |
| `Logger`                 | Return the configured `*slog.Logger`                     |
| `RequireDependency`      | Error if a single dependency is nil                      |
| `RequireAllDependencies` | Error on the first nil in a `map[string]any`             |
| `Errorf`                 | Formatted error (no wrapping)                            |
| `WrapError`              | Wrap error with `%w` for `errors.Is`/`errors.As` support |
