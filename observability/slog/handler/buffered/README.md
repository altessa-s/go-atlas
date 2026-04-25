# buffered

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/buffered"
```

Package `buffered` provides an asynchronous buffered `slog.Handler`. Records are written by a background goroutine; records at or above
the bypass level (default: `slog.LevelError`) are written synchronously to guarantee delivery.
