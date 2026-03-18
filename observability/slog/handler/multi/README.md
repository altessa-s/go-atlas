# multi

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/multi"
```

Package `multi` provides a `slog.Handler` that fans out log records to multiple child handlers. Each record is dispatched to every child
whose `Enabled` method returns true for the record's level. On Go 1.26+ this delegates to `slog.MultiHandler` from the standard library.

The handler is safe for concurrent use and immutable after creation — `WithAttrs` and `WithGroup` return new instances. Implements
`slogx.InnerHandlers` so `slogx.Shutdown` can traverse all children automatically.
