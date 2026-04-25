# context

```go
import "github.com/altessa-s/go-atlas/core/context"
```

Package `context` provides helpers for applying and guarding timeouts on `context.Context` values. All functions are nil-safe and thread-safe.

## Functions

| Function         | Description                                                          |
|------------------|----------------------------------------------------------------------|
| `ApplyTimeout`   | Add a timeout only when the context has no existing deadline         |
| `WithDefault`    | Like `ApplyTimeout` but treats a nil context as `context.Background` |
| `WithMaxTimeout` | Cap an existing deadline to a maximum duration                       |
| `OrBackground`   | Return the context as-is, or `context.Background` when nil           |

All functions return a safe-to-call `context.CancelFunc`, even when no new deadline is created, so callers can always `defer cancel()` safely.
