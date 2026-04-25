# factory

```go
import "github.com/altessa-s/go-atlas/observability/slog/factory"
```

Package `factory` provides a fluent builder for creating structured loggers from configuration.
`LoggerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
logger, err := factory.New(cfg.Logger).
    WithEnableMasking().
    Build()
```

After `Build`, the builder retains its level variable and can be used for runtime level changes.

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `LoggerBuilder` for the given logger config |

### Configuration

| Method | Description |
|--------|-------------|
| `WithPrefixKey(key)` | Sets the attribute key used for module prefix values |
| `WithPrefixColors(colors)` | Defines ANSI colors for prefix attributes in colorized output |
| `WithEnableMasking()` | Enables sensitive field masking using tags from config |
| `WithAppName(name)` | Overrides the application name included in log metadata |
| `WithAppVersion(version)` | Overrides the application version included in log metadata |
| `WithServiceId(id)` | Sets the service ID included in log metadata |
| `WithLevelVar(lv)` | Sets a custom dynamic log level variable |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the `*slog.Logger` |
| `SetLevel(level)` | Changes the logging level at runtime after `Build` |
| `GetLevel()` | Returns the current logging level |
