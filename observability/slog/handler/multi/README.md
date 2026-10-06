# multi

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/multi"
```

Package `multi` provides a `slog.Handler` that fans out log records to multiple child handlers. Each record is dispatched to every child
whose `Enabled` method returns true for the record's level. It delegates to `slog.MultiHandler` and exposes the children through `Handlers()`, which `slog.Shutdown` uses to drain them.

The handler is safe for concurrent use and immutable after creation — `WithAttrs` and `WithGroup` return new instances. Implements
`slogx.InnerHandlers` so `slogx.Shutdown` can traverse all children automatically.
