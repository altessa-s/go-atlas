# factory

```go
import "github.com/altessa-s/go-atlas/observability/slog/factory"
```

Package `factory` provides configuration-based creation of `slog.Logger` instances. Reads from `config.Logger`
to select the handler, format, level, and other settings. Supports runtime level changes and custom handlers.

## Usage

```go
f := factory.New()
logger, err := f.CreateLoggerFromConfig(&cfg.Logger)
if err != nil {
    return err
}

// Runtime level change
f.SetLevel(slog.LevelDebug)
```
