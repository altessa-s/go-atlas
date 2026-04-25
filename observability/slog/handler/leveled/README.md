# leveled

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/leveled"
```

Package `leveled` provides a `slog.Handler` middleware that filters log records based on per-subsystem log levels. When the subsystem is
captured via `WithAttrs` (i.e. the logger was created with `slogx.Module`), `Enabled` performs a single integer comparison (zero-cost fast
path). Otherwise, `Handle` scans record attributes to find the subsystem key.

## Options

| Option                | Default          | Description                                          |
|-----------------------|------------------|------------------------------------------------------|
| `WithDefaultLevel`    | `slog.LevelInfo` | Fallback level for subsystems not in the levels map  |
| `WithSubsystemLevels` | --               | Map of subsystem names to minimum log levels         |
| `WithSubsystemKey`    | `"subsystem"`    | Attribute key used to identify subsystems            |
