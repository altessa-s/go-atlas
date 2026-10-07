# buffered

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/buffered"
```

Package `buffered` provides an asynchronous buffered `slog.Handler`. Records are written by a background goroutine; records at or above
the bypass level (default: `slog.LevelError`) are written synchronously to guarantee delivery. Handlers derived via `WithAttrs` /
`WithGroup` (e.g. `logger.With(...)`) share the buffer and worker of their parent; `Shutdown` on any of them drains and stops it.
