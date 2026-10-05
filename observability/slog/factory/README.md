# factory

```go
import "github.com/altessa-s/go-atlas/observability/slog/factory"
```

Package `factory` provides a fluent builder for creating structured loggers from configuration.
`LoggerBuilder` reports configuration errors (a nil config, invalid `maskRules` entries) from `Build()`, before building anything.

## Quick Start

```go
logger, err := factory.New(cfg.Logger).
    WithEnableMasking().
    Build()
```

After `Build`, the builder retains its level variable and can be used for runtime level changes.

## Prefix key

The default prefix key `ModuleKey` equals `slogx.ModuleKey` (`"subsystem"`), so `logger.With(slogx.Module("auth"))` gets the `[auth]` tag
(`"subsystem":"auth"` in JSON) and the per-subsystem levels from `subsystems:`. The tag stays at the group level it was attached to, so a
later `WithGroup` does not move it. Several module values at one level are merged (`"auth:cache"`). The default used to be `"module"`; call
`WithPrefixKey("module")` to keep that behavior.

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `LoggerBuilder` for the given logger config |

### Configuration

| Method | Description |
|--------|-------------|
| `WithPrefixKey(key)` | Sets the attribute key used for module prefix values (default `ModuleKey` = `slogx.ModuleKey`, `"subsystem"`) |
| `WithPrefixColors(colors)` | Defines ANSI colors for prefix attributes in colorized output |
| `WithEnableMasking()` | Enables sensitive field masking using tags from config |
| `WithAppName(name)` | Overrides the application name included in log metadata |
| `WithAppVersion(version)` | Overrides the application version included in log metadata |
| `WithServiceId(id)` | Sets the service ID included in log metadata |
| `WithLevelVar(lv)` | Sets a custom dynamic log level variable |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the `*slog.Logger`; fails on a nil config or any invalid `maskRules` entry |
| `SetLevel(level)` | Changes the logging level at runtime after `Build` |
| `GetLevel()` | Returns the current logging level |
