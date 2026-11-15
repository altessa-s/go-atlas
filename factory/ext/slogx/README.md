# factory

```go
import "github.com/altessa-s/go-atlas/factory/ext/slogx"
```

Package `factory` maps the logger configuration to a logger assembled by [go-slogx `setup`](https://github.com/altessa-s/go-slogx/tree/main/setup).
`LoggerBuilder` reports configuration errors (a nil config, invalid `maskRules` entries) from `Build()`, before building anything.

## Quick Start

```go
logger, err := factory.New(cfg.Logger).
    WithEnableMasking().
    Build()
```

After `Build`, `SetLevel` and `GetLevel` act on the default level of the logger built last. With `buffer.enabled`, `Build` registers the
logger's `Shutdown` with `runtime.OnShutdown` to drain the buffer.

## Prefix key

The default prefix key `slogx.ModuleKey` from `github.com/altessa-s/go-slogx` equals `"subsystem"`, so `logger.With(slogx.Module("auth"))` gets the `[auth]` tag
(`"subsystem":"auth"` in JSON) and the per-subsystem levels from `subsystems:`. The tag stays at the group level it was attached to, so a later
`WithGroup` does not move it. Several module values at one level are merged (`"auth:cache"`). The default used to be `"module"`; call
`WithPrefixKey("module")` to keep that behavior.

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `LoggerBuilder` for the given logger config |

### Configuration

| Method | Description |
|--------|-------------|
| `WithPrefixKey(key)` | Sets the attribute key used for module prefix values (default `slogx.ModuleKey`, `"subsystem"`) |
| `WithEnableMasking()` | Enables sensitive field masking using tags from config |
| `WithAppName(name)` | Overrides the application name included in log metadata |
| `WithAppVersion(version)` | Overrides the application version included in log metadata |
| `WithServiceId(id)` | Sets the service ID included in log metadata |

### Mask types

`maskRules[].type` resolves against the go-slogx `mask` catalog (`full`, `email`, `phone`, `credit_card`, `url`, `partial`, `fixed`,
`pattern`), extended with `smart` (two runes kept on each side), `hash` (`prefix` + `[HASH]`), and the aliases `cc` and `cached_partial`.

### Handler registry

| Function | Description |
|----------|-------------|
| `RegisterHandler(format, factory)` | Registers a custom `HandlerFactory` for a log `outputFormat` |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the `*slog.Logger`; fails on a nil config or any invalid `maskRules` entry |
| `SetLevel(level)` | Changes the logging level at runtime after `Build` |
| `GetLevel()` | Returns the current logging level |
