# factory

```go
import "github.com/altessa-s/go-atlas/observability/slog/factory"
```

Package `factory` provides configuration-based creation of `slog.Logger` instances. Reads from `config.Logger` to select the handler,
format, level, and other settings. Supports runtime level changes via `SetLevel` and custom handler registration.
